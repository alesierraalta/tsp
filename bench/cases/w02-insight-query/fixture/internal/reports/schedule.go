package reports

import (
	"fmt"
	"time"
)

// Window is the period a report answers for: a start and an end, both inclusive of the start and
// exclusive of the end, in UTC. A window whose end is before its start is refused rather than
// swapped, because a swapped window answers a question the caller did not ask.
type Window struct {
	Start time.Time
	End   time.Time
}

// ParseWindow reads the "start,end" form the console sends, as RFC3339 timestamps.
func ParseWindow(raw string) (Window, error) {
	if raw == "" {
		return Window{}, fmt.Errorf("reports: an empty window")
	}
	start, end, ok := cut(raw, ",")
	if !ok {
		return Window{}, fmt.Errorf("reports: window %q is not start,end", raw)
	}
	from, err := time.Parse(time.RFC3339, start)
	if err != nil {
		return Window{}, fmt.Errorf("reports: window start %q: %v", start, err)
	}
	to, err := time.Parse(time.RFC3339, end)
	if err != nil {
		return Window{}, fmt.Errorf("reports: window end %q: %v", end, err)
	}
	return NewWindow(from, to)
}

// NewWindow validates a window.
func NewWindow(start, end time.Time) (Window, error) {
	if start.IsZero() || end.IsZero() {
		return Window{}, fmt.Errorf("reports: a window needs both ends")
	}
	if !end.After(start) {
		return Window{}, fmt.Errorf("reports: window ends at or before it starts")
	}
	if end.Sub(start) > MaxWindow {
		return Window{}, fmt.Errorf("reports: window is longer than %s", MaxWindow)
	}
	return Window{Start: start.UTC(), End: end.UTC()}, nil
}

// MaxWindow is the longest period a report answers for. A wider period is a bulk export, which is a
// different endpoint with its own limits.
const MaxWindow = 90 * 24 * time.Hour

// Contains reports whether an instant sits in the window.
func (w Window) Contains(at time.Time) bool {
	at = at.UTC()
	return !at.Before(w.Start) && at.Before(w.End)
}

// Days reports how many whole days the window spans, rounded up.
func (w Window) Days() int {
	span := w.End.Sub(w.Start)
	days := int(span / (24 * time.Hour))
	if span%(24*time.Hour) != 0 {
		days++
	}
	return days
}

// String renders the window in the form ParseWindow reads.
func (w Window) String() string {
	return w.Start.Format(time.RFC3339) + "," + w.End.Format(time.RFC3339)
}

// Last returns the window of the n periods before now, one period apart.
func Last(now time.Time, period time.Duration, n int) Window {
	if n <= 0 {
		n = 1
	}
	end := now.UTC().Truncate(period)
	start := end.Add(-time.Duration(n) * period)
	return Window{Start: start, End: end}
}

func cut(raw, sep string) (string, string, bool) {
	for i := 0; i+len(sep) <= len(raw); i++ {
		if raw[i:i+len(sep)] == sep {
			return raw[:i], raw[i+len(sep):], true
		}
	}
	return "", "", false
}
