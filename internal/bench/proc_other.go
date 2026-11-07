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

// runBounded runs a bounded command; with no process groups or /proc there is nothing left to end here, but a
// leftover child holding the output still must not turn a passing command into a failure.
func runBounded(cmd *exec.Cmd) error {
	return commandOutcome(cmd, cmd.Run())
}
