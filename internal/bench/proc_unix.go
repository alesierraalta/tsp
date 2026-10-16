//go:build linux || darwin

package bench

import (
	"os/exec"
	"syscall"
	"time"
)

// boundProcessTree makes a context timeout end the whole tree a suite started, not only its first process.
// node --test runs each test file in its own worker: killing the parent alone leaves a worker that loops on a
// planted defect running for hours, and it holds the output pipe open so Wait never returns. The suite gets
// its own process group, the cancel kills that group, and Wait stops waiting on the pipe shortly after.
func boundProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 2 * time.Second
}
