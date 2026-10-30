package setup

import (
	"bytes"
	"context"
	"os/exec"
	"testing"
	"time"
)

// A cancel ends a step whose shell waits on a child that holds the output pipe.
// Without the process group, Run waits the full minute of the child.
func TestCancelStopsTheChildrenOfAStep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 60 & wait")
	cmd.Stdout, cmd.Stderr = &out, &out
	stopWholeGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	time.AfterFunc(200*time.Millisecond, cancel)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("a cancelled step reported success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the cancel did not end the step")
	}
}
