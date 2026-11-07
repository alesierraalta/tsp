package bench

import (
	"errors"
	"os/exec"
)

// commandOutcome turns the error of a bounded run into the command's own outcome. When a leftover child still
// holds the output pipe, Wait gives up after WaitDelay and returns ErrWaitDelay, and it does so only once the
// command itself exited successfully: that is a passing run, not a failure with an unknown exit code. Read as
// a failure it turned a green suite into a broken one.
func commandOutcome(cmd *exec.Cmd, err error) error {
	if errors.Is(err, exec.ErrWaitDelay) && cmd.ProcessState != nil && cmd.ProcessState.Success() {
		return nil
	}
	return err
}
