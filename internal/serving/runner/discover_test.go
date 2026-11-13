package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestParseServe(t *testing.T) {
	cases := []struct {
		cmd   string
		ok    bool
		port  int
		model string
	}{
		{"/opt/venv/bin/vllm serve Qwen/Qwen3-8B --port 8001", true, 8001, "Qwen/Qwen3-8B"},
		{"vllm serve /models/q27 --served-model-name q27 --port=9000", true, 9000, "q27"},
		{"python3 -m vllm.entrypoints.openai.api_server --model org/m --host 0.0.0.0", true, 8000, "org/m"},
		{"python -m vllm.entrypoints.cli.main serve org/m --tensor-parallel-size 4", true, 8000, "org/m"},
		{"vllm bench serve --port 8001", false, 0, ""},
		{"/usr/bin/python3 server.py --port 8000", false, 0, ""},
	}
	for _, c := range cases {
		a, ok := parseServe(strings.Fields(c.cmd))
		if ok != c.ok || (ok && (a.port != c.port || a.model != c.model)) {
			t.Errorf("%q: ok %v port %d model %q", c.cmd, ok, a.port, a.model)
		}
	}
}

// writeProc writes a fake /proc/<pid>/cmdline.
func writeProc(t *testing.T, root string, pid int, argv ...string) {
	t.Helper()
	dir := filepath.Join(root, "proc", strconv.Itoa(pid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmdline"), []byte(strings.Join(argv, "\x00")+"\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverFindsProcessesAndContainers(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	live, _ := strconv.Atoi(u.Port())

	root := t.TempDir()
	writeProc(t, root, 100, "/venv/bin/vllm", "serve", "org/live", "--port", u.Port())
	writeProc(t, root, 101, "/venv/bin/vllm", "serve", "org/dead", "--port", "1")
	writeProc(t, root, 102, "bash", "-c", "sleep 10")
	old := listContainers
	t.Cleanup(func() { listContainers = old })
	listContainers = func(_ context.Context, engine string) (string, error) {
		if engine != "docker" {
			return "", nil
		}
		return "vllm-a\tvllm/vllm-openai:latest\t\"--model org/c --port 8000\"\t0.0.0.0:18555->8000/tcp\n" +
			"web\tnginx\t\"nginx -g daemon off;\"\t0.0.0.0:80->80/tcp\n" +
			"vllm-host\trocm/vllm\t\"vllm serve org/h --port 18556\"\t\n", nil
	}
	found, err := Discover(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int]Found{}
	for _, f := range found {
		got[f.Port] = f
	}
	if len(found) != 4 {
		t.Fatalf("found %+v, want 4 servers", found)
	}
	if f := got[live]; f.Model != "org/live" || f.Source != "process" || f.PID != 100 || !f.Healthy {
		t.Errorf("the live process: %+v", f)
	}
	if f := got[1]; f.Healthy {
		t.Errorf("a server that does not answer is healthy: %+v", f)
	}
	if f := got[18555]; f.Container != "vllm-a" || f.Model != "org/c" || f.Source != "docker" {
		t.Errorf("the published container: %+v", f)
	}
	if f := got[18556]; f.Container != "vllm-host" || f.Model != "org/h" {
		t.Errorf("the host-network container: %+v", f)
	}

	// A server that bernie already records names its run.
	rec := &Record{Name: "mine", Host: "127.0.0.1", Port: live, Foreign: true}
	if err := writeRecord(rec); err != nil {
		t.Fatal(err)
	}
	found, _ = Discover(context.Background(), root)
	for _, f := range found {
		if f.Port == live && f.Run != "mine" {
			t.Errorf("the recorded server names run %q, want mine", f.Run)
		}
	}
}
