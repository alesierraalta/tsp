// Package clock is the artifact server's only source of time.
package clock

import "time"

// Clock returns the instant a caller treats as now.
type Clock func() time.Time

// System is the wall clock, read in UTC.
func System() Clock { return func() time.Time { return time.Now().UTC() } }

// Fixed returns a clock that always reports t.
func Fixed(t time.Time) Clock {
	at := t.UTC()
	return func() time.Time { return at }
}

// Day truncates t to the midnight of its UTC day.
func Day(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// DayNumber returns the UTC day t falls on as a count of days since the epoch.
func DayNumber(t time.Time) int64 { return Day(t).Unix() / 86400 }

// DaysBetween returns the whole UTC days between two instants, signed.
func DaysBetween(from, to time.Time) int64 { return DayNumber(to) - DayNumber(from) }

// Format renders an instant the way a response carries it.
func Format(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// Parse reads an instant an operator wrote.
func Parse(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// Window is a half-open window of instants.
type Window struct {
	Start time.Time
	End   time.Time
}

// NewWindow returns the window between two instants, refusing an empty one.
func NewWindow(start, end time.Time) (Window, error) {
	if !end.After(start) {
		return Window{}, ErrEmptyWindow
	}
	return Window{Start: start.UTC(), End: end.UTC()}, nil
}

// Contains reports whether t is inside the window.
func (w Window) Contains(t time.Time) bool {
	t = t.UTC()
	return !t.Before(w.Start) && t.Before(w.End)
}

// Days returns the whole UTC days the window covers.
func (w Window) Days() int64 { return DaysBetween(w.Start, w.End) }

// String renders the window.
func (w Window) String() string {
	return w.Start.Format(time.DateOnly) + ".." + w.End.Format(time.DateOnly)
}
