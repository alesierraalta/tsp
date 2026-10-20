//go:build windows

package plan

import (
	"os"

	"github.com/alesierraalta/tsp/internal/filelock"
)

func lockFile(file *os.File) error   { return filelock.Lock(file) }
func unlockFile(file *os.File) error { return filelock.Unlock(file) }
