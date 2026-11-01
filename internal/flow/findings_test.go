package flow

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/serving/vllm/vllmtest"
)

const oomLog = `# bernie 2026-10-28T10:00:00Z: python -m vllm.entrypoints.cli.main serve org/m
INFO 10-28 10:00:05 [loader.py:200] Loading weights
(EngineCore pid=816631) ERROR 09-30 13:04:08 [core.py:1383] torch.OutOfMemoryError: CUDA out of memory. Tried to allocate 772.00 MiB. GPU 0 has a total capacity of 29.98 GiB of which 38.00 MiB is free.
`

const plainLog = `# bernie 2026-10-28T10:00:00Z: python -m vllm.entrypoints.cli.main serve org/m
INFO 10-28 10:00:05 [loader.py:200] Loading weights
Killed
`

// writeRun records a host run named name with the given log and pid.
func writeRun(t *testing.T, name, log string, pid, port int) {
	t.Helper()
	dir := runner.RunDir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "serve.log")
	if err := os.WriteFile(logPath, []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := runner.Record{Name: name, Recipe: "r", Mode: "host", PID: pid, Host: "127.0.0.1", Port: port,
		StartedAt: time.Now().UTC().Truncate(time.Second), Log: logPath}
	b, _ := json.Marshal(rec)
	if err := os.WriteFile(filepath.Join(dir, "run.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// liveVLLM starts a process whose command line names vllm (what the runner
// checks for) and a fake vLLM server on port: it answers /health and a
// completion, so the run is running.
func liveVLLM(t *testing.T) (pid, port int) {
	t.Helper()
	cmd := exec.Command("sh", "-c", "sleep 60; :", "vllm")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	srv := httptest.NewServer(vllmtest.New(vllmtest.Config{}))
	t.Cleanup(srv.Close)
	_, p, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	port, _ = strconv.Atoi(p)
	return cmd.Process.Pid, port
}

func stateEnv(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
}

func TestRunsFindingsOfExitedRuns(t *testing.T) {
	stateEnv(t)
	writeRun(t, "oom", oomLog, 0, 18000)
	writeRun(t, "plain", plainLog, 0, 18001)
	pid, port := liveVLLM(t)
	writeRun(t, "live", oomLog, pid, port) // a running run is not scanned

	runs, err := Runs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Run{}
	for _, r := range runs {
		got[r.Name] = r
	}
	if r := got["oom"]; r.State != runner.StateExited || len(r.Findings) != 1 || r.Findings[0].ID != "memory.gpu-oom" || r.Findings[0].LineNo != 3 {
		t.Errorf("oom run = %s, %+v", r.State, r.Findings)
	}
	if r := got["plain"]; r.State != runner.StateExited || len(r.Findings) != 0 {
		t.Errorf("plain run = %s, %+v", r.State, r.Findings)
	}
	if r := got["live"]; r.State != runner.StateRunning || len(r.Findings) != 0 {
		t.Errorf("live run = %s, %+v", r.State, r.Findings)
	}

	// ps --json: findings only on the exited run that matched.
	b, err := json.Marshal(runs)
	if err != nil {
		t.Fatal(err)
	}
	var doc []map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	for _, e := range doc {
		f, has := e["findings"]
		switch e["name"] {
		case "oom":
			l, _ := f.([]any)
			if len(l) != 1 || l[0].(map[string]any)["id"] != "memory.gpu-oom" || l[0].(map[string]any)["fix"] == "" {
				t.Errorf("oom findings = %v", f)
			}
		default:
			if has {
				t.Errorf("%v has findings %v", e["name"], f)
			}
		}
	}
}

func TestWaitHealthyReportsStartupExit(t *testing.T) {
	stateEnv(t)
	writeRun(t, "oom", oomLog, 0, 18000)
	writeRun(t, "plain", plainLog, 0, 18001)

	var se *StartupExit
	err := WaitHealthy(context.Background(), "oom", time.Minute, nil)
	if !errors.As(err, &se) || se.Name != "oom" || len(se.Findings) != 1 || se.Findings[0].ID != "memory.gpu-oom" {
		t.Fatalf("WaitHealthy(oom) = %v", err)
	}
	if want := "oom exited during startup; see: bernie logs oom"; err.Error() != want {
		t.Errorf("message = %q, want %q", err, want)
	}
	err = WaitHealthy(context.Background(), "plain", time.Minute, nil)
	if !errors.As(err, &se) || len(se.Findings) != 0 || err.Error() != "plain exited during startup; see: bernie logs plain" {
		t.Errorf("WaitHealthy(plain) = %v (%+v)", err, se)
	}
}

func TestWaitHealthyRunningIsNil(t *testing.T) {
	stateEnv(t)
	pid, port := liveVLLM(t)
	writeRun(t, "live", oomLog, pid, port)
	if err := WaitHealthy(context.Background(), "live", time.Minute, nil); err != nil {
		t.Fatal(err)
	}
}

func TestFindingsGivesUpWhenContextEnds(t *testing.T) {
	stateEnv(t)
	writeRun(t, "oom", oomLog, 0, 18000)
	rec, err := runner.Get("oom")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := Findings(ctx, *rec); len(got) != 0 {
		t.Errorf("findings with a done context = %+v", got)
	}
	// The cut-off scan was not cached: the next one sees the log.
	if got := Findings(context.Background(), *rec); len(got) != 1 {
		t.Errorf("findings = %+v", got)
	}
}
