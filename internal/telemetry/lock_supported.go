//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || windows

package telemetry

// lockUnavailable reports whether a platform's file lock is the deliberate refusal of an unsupported
// system; on every platform with a lock the answer is no, so a failed lock is a real error.
func lockUnavailable(error) bool { return false }
