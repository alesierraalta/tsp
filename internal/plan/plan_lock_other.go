//go:build !(darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || windows)

package plan

import (
	"os"

	"github.com/alesierraalta/tsp/internal/filelock"
)

var errNoPlanLock = filelock.ErrUnsupported

func lockFile(file *os.File) error   { return filelock.Lock(file) }
func unlockFile(file *os.File) error { return filelock.Unlock(file) }
