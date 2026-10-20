//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd

package filelock

import (
	"os"
	"syscall"
)

// Lock waits for an exclusive advisory lock. The kernel releases it on process exit.
// Never unlink a lock file: waiters must continue to share the same inode.
func Lock(file *os.File) error { return syscall.Flock(int(file.Fd()), syscall.LOCK_EX) }

func Unlock(file *os.File) error { return syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }
