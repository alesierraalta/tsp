//go:build !(darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || windows)

package filelock

import (
	"errors"
	"os"
	"runtime"
)

var ErrUnsupported = errors.New("tsp has no cross-process file lock on " + runtime.GOOS + ", so it refuses to write a plan without serialization")

func Lock(*os.File) error   { return ErrUnsupported }
func Unlock(*os.File) error { return ErrUnsupported }
