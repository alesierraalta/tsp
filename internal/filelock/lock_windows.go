//go:build windows

package filelock

import (
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

const lockfileExclusiveLock = 0x00000002

// Lock waits for an exclusive whole-file lock, released on close or process exit.
func Lock(file *os.File) error {
	var overlapped syscall.Overlapped
	r1, _, err := procLockFileEx.Call(uintptr(file.Fd()), lockfileExclusiveLock, 0, ^uintptr(0), ^uintptr(0), uintptr(unsafe.Pointer(&overlapped)))
	runtime.KeepAlive(&overlapped)
	if r1 == 0 {
		return err
	}
	return nil
}

func Unlock(file *os.File) error {
	var overlapped syscall.Overlapped
	r1, _, err := procUnlockFileEx.Call(uintptr(file.Fd()), 0, ^uintptr(0), ^uintptr(0), uintptr(unsafe.Pointer(&overlapped)))
	runtime.KeepAlive(&overlapped)
	if r1 == 0 {
		return err
	}
	return nil
}
