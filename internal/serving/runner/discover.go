package runner

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opengfx1030/bernie/internal/support/execx"
)

// Found is a vLLM server that runs on this host, found by Discover (spec
// §8.5). Run names the run that already records it; an empty Run means
// bernie does not know the server yet and `bernie attach URL` adds it.
type Found struct {
	URL       string `json:"url"`
	Port      int    `json:"port"`
	Model     string `json:"model,omitempty"`
	Source    string `json:"source"` // process, docker or podman
	PID       int    `json:"pid,omitempty"`
	Container string `json:"container,omitempty"`
	Healthy   bool   `json:"healthy"`
	Run       string `json:"run,omitempty"`
}

// foundArgs are the parts of a vLLM command line that say where the
// server listens and what it serves.
type foundArgs struct {
	port  int
	model string
}

// parseServe reads a vLLM command line: `vllm serve MODEL`, the
// openai.api_server module or the cli module, with --port, --model and
// --served-model-name. ok is false for any other command.
func parseServe(argv []string) (foundArgs, bool) {
	a := foundArgs{port: 8000}
	ok := false
	for i, arg := range argv {
		switch {
		case filepath.Base(arg) == "vllm" && i+1 < len(argv) && argv[i+1] == "serve":
			ok = true
			if i+2 < len(argv) && !strings.HasPrefix(argv[i+2], "-") {
				a.model = argv[i+2]
			}
		case strings.Contains(arg, "vllm.entrypoints.openai.api_server"):
			ok = true
		case strings.Contains(arg, "vllm.entrypoints.cli.main") && i+1 < len(argv) && argv[i+1] == "serve":
			ok = true
			if i+2 < len(argv) && !strings.HasPrefix(argv[i+2], "-") {
				a.model = argv[i+2]
			}
		}
	}
	if !ok {
		return a, false
	}
	served := ""
	for i := 0; i < len(argv); i++ {
		name, val, inline := strings.Cut(argv[i], "=")
		if !inline && i+1 < len(argv) {
			val = argv[i+1]
		}
		switch name {
		case "--port":
			if n, err := strconv.Atoi(val); err == nil && n > 0 && n < 65536 {
				a.port = n
			}
		case "--model":
			a.model = val
		case "--served-model-name":
			served = val
		}
	}
	if served != "" {
		a.model = served
	}
	return a, true
}

// scanProcesses reads <root>/proc/*/cmdline for vLLM servers.
func scanProcesses(root string) []Found {
	dirs, _ := filepath.Glob(filepath.Join(root, "proc", "[0-9]*"))
	var out []Found
	for _, d := range dirs {
		b, err := os.ReadFile(filepath.Join(d, "cmdline"))
		if err != nil || len(b) == 0 {
			continue
		}
		argv := strings.Split(strings.TrimRight(string(b), "\x00"), "\x00")
		a, ok := parseServe(argv)
		if !ok {
			continue
		}
		pid, _ := strconv.Atoi(filepath.Base(d))
		out = append(out, Found{Port: a.port, Model: a.model, Source: "process", PID: pid})
	}
	return out
}

// listContainers is `<engine> ps` with the name, the image, the command
// and the ports of every running container; tests replace it.
var listContainers = func(ctx context.Context, engine string) (string, error) {
	if execx.Which(engine) == "" {
		return "", nil
	}
	r, err := execx.Run(ctx, 5*time.Second, engine, "ps", "--no-trunc", "--format", "{{.Names}}\t{{.Image}}\t{{.Command}}\t{{.Ports}}")
	if err != nil {
		return "", err
	}
	if !r.OK() {
		return "", fmt.Errorf("%s ps: %s", engine, execx.FirstLine(r.Stderr))
	}
	return r.Stdout, nil
}

var publishRE = regexp.MustCompile(`:(\d+)->(\d+)/tcp`)

// scanContainers finds the containers whose image or command is vLLM. A
// published port maps to the host; a container on the host network
// listens on the port of its command.
func scanContainers(ctx context.Context) []Found {
	var out []Found
	for _, engine := range []string{"docker", "podman"} {
		text, err := listContainers(ctx, engine)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(text, "\n") {
			f := strings.Split(line, "\t")
			if len(f) < 4 || !strings.Contains(strings.ToLower(f[1]+" "+f[2]), "vllm") {
				continue
			}
			cmd := strings.Fields(strings.Trim(f[2], `"`))
			a, ok := parseServe(append([]string{"vllm", "serve"}, cmd...))
			if !ok {
				continue
			}
			port := a.port
			for _, m := range publishRE.FindAllStringSubmatch(f[3], -1) {
				if m[2] == strconv.Itoa(a.port) {
					port, _ = strconv.Atoi(m[1])
					break
				}
			}
			out = append(out, Found{Port: port, Model: a.model, Source: engine, Container: f[0]})
		}
	}
	return out
}

// Discover finds the vLLM servers of this host: the processes under
// <root>/proc and the containers of docker and podman. It only reads: it
// starts, stops and records nothing. Every server is checked on /health of
// 127.0.0.1 and matched with the runs bernie records.
func Discover(ctx context.Context, root string) ([]Found, error) {
	byPort := map[int]Found{}
	for _, f := range append(scanProcesses(root), scanContainers(ctx)...) {
		// A container's server also shows as a process: the container
		// entry, which knows the published port, wins.
		if prev, ok := byPort[f.Port]; ok && prev.Container != "" {
			continue
		}
		byPort[f.Port] = f
	}
	runs, _ := List(ctx)
	out := make([]Found, 0, len(byPort))
	for _, f := range byPort {
		f.URL = "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(f.Port))
		for _, r := range runs {
			if r.State != StateExited && r.Port == f.Port && (r.Host == "" || isLocal(r.Host)) {
				f.Run = r.Name
			}
		}
		out = append(out, f)
	}
	var wg sync.WaitGroup
	for i := range out {
		wg.Add(1)
		go func(f *Found) {
			defer wg.Done()
			f.Healthy = healthy(ctx, f.URL)
		}(&out[i])
	}
	wg.Wait()
	sort.Slice(out, func(i, j int) bool { return out[i].Port < out[j].Port })
	return out, nil
}

func isLocal(h string) bool {
	switch h {
	case "127.0.0.1", "localhost", "::1", "0.0.0.0", "::", "[::]":
		return true
	}
	return false
}
