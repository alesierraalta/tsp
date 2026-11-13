package clock

import "errors"

// Errors a window reports.
var (
	// ErrEmptyWindow is returned for a window whose end is not after its start.
	ErrEmptyWindow = errors.New("clock: the window is empty")
)
