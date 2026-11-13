package clock

import "errors"

// Errors a cycle or a period reports.
var (
	// ErrEmptyPeriod is returned for a period whose end is not after its start.
	ErrEmptyPeriod = errors.New("clock: the period is empty")
	// ErrCycleLength is returned for a cycle of zero or a negative number of months.
	ErrCycleLength = errors.New("clock: a cycle needs a positive number of months")
)
