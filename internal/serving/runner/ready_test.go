package runner

import (
	"context"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/vllm/vllmtest"
)

// foreignRun is a run record of an attached server at srv: alive as long
// as it answers, so the state depends on /health and the completion only.
func foreignRun(t *testing.T, srv *httptest.Server) *Record {
	t.Helper()
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	host, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	p, _ := strconv.Atoi(port)
	r := &Record{Name: "ready", Foreign: true, Host: host, Port: p, StartedAt: time.Now()}
	if err := os.MkdirAll(RunDir(r.Name), 0o755); err != nil {
		t.Fatal(err)
	}
	return r
}

// A server that answers /health but no completion (the model is not
// loaded) stays starting and says why.
func TestRunWithoutAnswerStaysStarting(t *testing.T) {
	fake := vllmtest.New(vllmtest.Config{FailEvery: 1})
	srv := httptest.NewServer(fake)
	defer srv.Close()
	r := foreignRun(t, srv)
	st, why := liveState(context.Background(), r)
	if st != StateStarting {
		t.Fatalf("state = %s, want starting", st)
	}
	if !strings.Contains(why, "answers /health but not a completion") {
		t.Errorf("waiting = %q, want the completion reason", why)
	}
	if _, err := os.Stat(filepath.Join(RunDir(r.Name), "answered")); err == nil {
		t.Error("a run that did not answer was recorded as answered")
	}
}

// A server that answers a completion is running, and the answer is asked
// once per run: a new start of the same name asks again.
func TestRunThatAnswersIsRunning(t *testing.T) {
	fake := vllmtest.New(vllmtest.Config{})
	srv := httptest.NewServer(fake)
	defer srv.Close()
	r := foreignRun(t, srv)
	for i := 0; i < 3; i++ {
		if st, why := liveState(context.Background(), r); st != StateRunning {
			t.Fatalf("poll %d: state = %s (%s), want running", i, st, why)
		}
	}
	if n := len(fake.Prompts()); n != 1 {
		t.Errorf("%d completions for three polls, want 1", n)
	}
	r.StartedAt = r.StartedAt.Add(time.Minute)
	if st, _ := liveState(context.Background(), r); st != StateRunning {
		t.Fatalf("restart: state = %s", st)
	}
	if n := len(fake.Prompts()); n != 2 {
		t.Errorf("%d completions after a restart, want 2", n)
	}
}

func TestRunWithoutHealthIsStarting(t *testing.T) {
	srv := httptest.NewServer(nil) // 404 for every path
	defer srv.Close()
	r := foreignRun(t, srv)
	if st, why := liveState(context.Background(), r); st != StateStarting || !strings.Contains(why, "/health") {
		t.Errorf("state = %s (%s), want starting on /health", st, why)
	}
}
