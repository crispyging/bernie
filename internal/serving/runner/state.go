package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/support/paths"
)

// List returns every recorded run with its state, oldest first.
func List(ctx context.Context) ([]Status, error) {
	dirs, err := filepath.Glob(filepath.Join(paths.State(), "runs", "*", "run.json"))
	if err != nil {
		return nil, err
	}
	out := []Status{}
	for _, f := range dirs {
		r, err := readRecord(filepath.Base(filepath.Dir(f)))
		if err != nil {
			continue
		}
		st, why := liveState(ctx, r)
		out = append(out, Status{Record: *r, State: st, Waiting: why})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	return out, nil
}

// state is the state of the run alone (see liveState).
func state(ctx context.Context, r *Record) string {
	st, _ := liveState(ctx, r)
	return st
}

// liveState is running (/health answers and the server answered a one-token
// completion), starting (alive, not yet both) or exited. For a starting
// run, the second result says what the run waits for (spec §8.3).
func liveState(ctx context.Context, r *Record) (string, string) {
	alive := false
	switch {
	case r.Foreign:
		// A foreign server has no process of ours to ask. It answers or
		// it does not.
		alive = true
	case r.Mode == recipe.ModeContainer:
		s, err := engineOut(ctx, r.Engine, "inspect", "--format", "{{.State.Running}}", r.Container)
		alive = err == nil && s == "true"
	default:
		alive = processAlive(r.PID)
	}
	if !alive {
		return StateExited, ""
	}
	if !healthy(ctx, r.URL()) {
		return StateStarting, "the server does not answer /health yet"
	}
	if err := answered(ctx, r); err != nil {
		return StateStarting, "the server answers /health but not a completion yet: " + err.Error()
	}
	return StateRunning, ""
}

func healthy(ctx context.Context, base string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// processAlive reports a live process; on Linux it also checks that the
// pid still runs vLLM (pids are reused).
func processAlive(pid int) bool {
	if pid <= 0 || syscall.Kill(pid, 0) != nil && !errors.Is(syscall.Kill(pid, 0), syscall.EPERM) {
		return false
	}
	if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)); err == nil {
		return bytes.Contains(b, []byte("vllm"))
	}
	if st, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil && bytes.Contains(st, []byte(") Z ")) {
		return false
	}
	return true
}

// resolveName returns name, or the only run that is not exited.
func resolveName(ctx context.Context, name string) (*Record, error) {
	if name != "" {
		r, err := readRecord(name)
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("no run named %s", name)
		}
		return r, err
	}
	runs, err := List(ctx)
	if err != nil {
		return nil, err
	}
	var live []Status
	for _, s := range runs {
		if s.State != StateExited {
			live = append(live, s)
		}
	}
	switch len(live) {
	case 1:
		return &live[0].Record, nil
	case 0:
		if len(runs) == 1 {
			return &runs[0].Record, nil
		}
		return nil, fmt.Errorf("no running run; give a NAME (bernie ps)")
	}
	names := make([]string, len(live))
	for i, s := range live {
		names[i] = s.Name
	}
	return nil, fmt.Errorf("several runs are running (%s); give a NAME", strings.Join(names, ", "))
}

// Stop stops a run (the only running one when name is ""): SIGTERM to the
// process group, 20 s, then SIGKILL; or `<engine> stop` and `rm`. A foreign
// run (one bernie attached, R29) is never stopped: Stop only forgets it.
func Stop(ctx context.Context, name string) error {
	r, err := resolveName(ctx, name)
	if err != nil {
		return err
	}
	if r.Foreign {
		return Forget(r.Name)
	}
	if r.Mode == recipe.ModeContainer {
		if _, err := engineOut(ctx, r.Engine, "stop", "-t", strconv.Itoa(int(StopTimeout/time.Second)), r.Container); err != nil {
			if !strings.Contains(err.Error(), "no such container") {
				return err
			}
		}
		_, _ = engineOut(ctx, r.Engine, "rm", "-f", r.Container)
		return nil
	}
	if !processAlive(r.PID) {
		return nil
	}
	return stopGroup(r.PID, StopTimeout)
}

func stopGroup(pid int, grace time.Duration) error {
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("SIGTERM process group %d: %w", pid, err)
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if syscall.Kill(-pid, 0) != nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("SIGKILL process group %d: %w", pid, err)
	}
	return nil
}
