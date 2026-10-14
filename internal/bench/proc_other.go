//go:build !linux && !darwin

package bench

import (
	"os/exec"
	"time"
)

// boundProcessTree has no process groups to kill here; it still stops Wait from blocking on a pipe a
// surviving child holds open.
func boundProcessTree(cmd *exec.Cmd) {
	cmd.WaitDelay = 2 * time.Second
}
