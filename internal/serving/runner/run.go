package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/serving/tuning"
	"github.com/opengfx1030/bernie/internal/support/paths"
)

// Run states (spec §8.3).
const (
	StateRunning  = "running"
	StateStarting = "starting"
	StateExited   = "exited"
)

// StopTimeout is the SIGTERM grace period before SIGKILL.
const StopTimeout = 20 * time.Second

// Record is <state>/runs/<name>/run.json.
type Record struct {
	Name        string       `json:"name"`
	Recipe      string       `json:"recipe"`
	Mode        string       `json:"mode"`
	PID         int          `json:"pid"`
	Container   string       `json:"container"`
	Engine      string       `json:"engine"`
	Host        string       `json:"host"`
	Port        int          `json:"port"`
	StartedAt   time.Time    `json:"started_at"`
	Log         string       `json:"log"`
	Image       string       `json:"image"`
	ImageDigest string       `json:"image_digest"`
	Plan        *recipe.Plan `json:"plan"`
	// Foreign is true when bernie did not start this server: it was
	// attached with `bernie attach` (R29). bernie never stops a foreign
	// server; Stop only forgets the record.
	Foreign bool `json:"foreign,omitempty"`
}

// Status is a record with its live state.
type Status struct {
	Record
	State string `json:"state"`
	// Waiting says why a starting run is not running yet (spec §8.3).
	Waiting string `json:"waiting,omitempty"`
}

// URL is the base URL of the run's server (loopback for wildcard hosts).
func (r Record) URL() string {
	h := r.Host
	switch h {
	case "", "0.0.0.0", "::", "[::]":
		h = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(h, strconv.Itoa(r.Port))
}

func recordPath(name string) string { return filepath.Join(RunDir(name), "run.json") }

func readRecord(name string) (*Record, error) {
	b, err := os.ReadFile(recordPath(name))
	if err != nil {
		return nil, err
	}
	var r Record
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", recordPath(name), err)
	}
	return &r, nil
}

// WriteRecord saves the record of a run. The demo mode uses it: the fake
// server of the demo writes no log of its own, so the demo points the
// record at the log it writes.
func WriteRecord(r *Record) error { return writeRecord(r) }

func writeRecord(r *Record) error {
	if _, err := paths.Ensure(RunDir(r.Name)); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := recordPath(r.Name) + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, recordPath(r.Name))
}

// Start starts a plan: a detached host process in a new session, or
// `<engine> run -d`. It refuses a name that is running. With Foreground it
// waits for the process to exit (Ctrl-C stops it).
func Start(ctx context.Context, p *recipe.Plan, o StartOptions) (*Record, error) {
	if prev, err := readRecord(p.Name); err == nil {
		if st := state(ctx, prev); st != StateExited {
			return nil, fmt.Errorf("run %s is %s (pid %d %s); stop it first (bernie stop %s) or use --name", p.Name, st, prev.PID, prev.Container, p.Name)
		}
	}
	if err := checkPort(ctx, p); err != nil {
		return nil, err
	}
	dir, err := paths.Ensure(RunDir(p.Name))
	if err != nil {
		return nil, err
	}
	rec := &Record{Name: p.Name, Recipe: p.Recipe, Mode: p.Mode, Host: p.Host, Port: p.Port, Plan: p,
		StartedAt: time.Now().UTC().Truncate(time.Second)}
	if p.Mode == recipe.ModeContainer {
		return startContainer(ctx, p, o, rec)
	}
	return startHost(ctx, p, o, rec, dir)
}

// checkPort refuses a plan whose port another process already listens on:
// the server dies with "Address already in use" and the operator waits for
// a server that never answers. The message names the run that holds the
// port when it is one of ours.
func checkPort(ctx context.Context, p *recipe.Plan) error {
	if p.Port <= 0 {
		return nil
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(p.Host, strconv.Itoa(p.Port)))
	if err == nil {
		_ = ln.Close()
		return nil
	}
	if !errors.Is(err, syscall.EADDRINUSE) {
		return nil
	}
	if name := runOnPort(ctx, p.Port); name != "" {
		return fmt.Errorf("port %d is already in use by the run %s; stop it first (bernie stop %s) or choose another port (--set args.port=%d)",
			p.Port, name, name, p.Port+1)
	}
	return fmt.Errorf("port %d is already in use; if it is a vLLM server, use it as it is (bernie attach %d), else stop the process that holds it or choose another port (--set args.port=%d)", p.Port, p.Port, p.Port+1)
}

// runOnPort returns the name of the run that listens on the port, or "".
func runOnPort(ctx context.Context, port int) string {
	recs, err := List(ctx)
	if err != nil {
		return ""
	}
	for _, r := range recs {
		if r.Port == port && r.State != StateExited {
			return r.Name
		}
	}
	return ""
}

func startHost(ctx context.Context, p *recipe.Plan, o StartOptions, rec *Record, dir string) (*Record, error) {
	ex, err := Command(p, o)
	if err != nil {
		return nil, err
	}
	for _, c := range cacheEnv {
		if _, err := paths.Ensure(filepath.Join(CacheDir(p.Name), c[1])); err != nil {
			return nil, err
		}
	}
	if f := p.Env["PYTORCH_TUNABLEOP_FILENAME"]; f == tuning.PerUserFile() {
		if _, err := paths.Ensure(filepath.Dir(f)); err != nil {
			return nil, err
		}
	}
	rec.Log = filepath.Join(dir, "serve.log")
	logf, err := os.OpenFile(rec.Log, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, err
	}
	defer logf.Close()
	fmt.Fprintf(logf, "# bernie %s: %s\n", rec.StartedAt.Format(time.RFC3339), shellJoin(ex.Argv))

	cmd := exec.Command(ex.Argv[0], ex.Argv[1:]...)
	cmd.Dir = ex.Dir
	cmd.Env = mergeEnv(os.Environ(), ex.Env, ex.Unset)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var out io.Writer = logf
	if o.Foreground && o.Progress != nil {
		out = io.MultiWriter(logf, o.Progress)
	}
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", ex.Argv[0], err)
	}
	rec.PID = cmd.Process.Pid
	if err := writeRecord(rec); err != nil {
		_ = syscall.Kill(-rec.PID, syscall.SIGTERM)
		return nil, err
	}
	if !o.Foreground {
		return rec, cmd.Process.Release()
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return rec, exitErr(err)
	case <-ctx.Done():
		_ = stopGroup(rec.PID, StopTimeout)
		<-done
		return rec, nil
	}
}

func exitErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("vLLM exited: %w", err)
}

// mergeEnv removes unset and overridden keys from base and appends add.
func mergeEnv(base, add, unset []string) []string {
	drop := map[string]bool{}
	for _, k := range unset {
		drop[k] = true
	}
	for _, kv := range add {
		k, _, _ := strings.Cut(kv, "=")
		drop[k] = true
	}
	out := make([]string, 0, len(base)+len(add))
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		if !drop[k] {
			out = append(out, kv)
		}
	}
	return append(out, add...)
}
