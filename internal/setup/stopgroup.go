package setup

import (
	"os/exec"
	"syscall"
	"time"
)

// killAfter is the time a cancelled step gets between SIGTERM and SIGKILL.
const killAfter = 10 * time.Second

// stopWholeGroup makes a cancel reach everything a step started. A step is a
// shell that runs other programs. Killing the shell alone leaves them holding
// the output pipe, and Run then waits for them: the Stop control appears to do
// nothing. The step runs in its own process group; a cancel signals the group,
// and WaitDelay bounds the wait whatever the children do.
func stopWholeGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		pgid := cmd.Process.Pid
		time.AfterFunc(killAfter, func() { _ = syscall.Kill(-pgid, syscall.SIGKILL) })
		return syscall.Kill(-pgid, syscall.SIGTERM)
	}
	cmd.WaitDelay = killAfter + 2*time.Second
}
