//go:build !(darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || windows)

package telemetry

import (
	"errors"

	"github.com/alesierraalta/tsp/internal/filelock"
)

// lockUnavailable reports the deliberate refusal of a platform without a file lock, which the ledger
// still writes through: a local event no other process is racing for is not worth losing.
func lockUnavailable(err error) bool { return errors.Is(err, filelock.ErrUnsupported) }
