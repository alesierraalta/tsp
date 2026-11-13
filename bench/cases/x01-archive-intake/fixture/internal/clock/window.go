package clock

import (
	"fmt"
	"time"
)

// Window is a closed-open range of instants: it contains from and excludes until. The pages and the
// retention sweep both work in windows, so a batch that closed exactly at the end instant belongs to the
// next window and is counted once.
type Window struct {
	From  time.Time
	Until time.Time
}

// DayWindow is the UTC day t belongs to.
func DayWindow(t time.Time) Window {
	start := Day(t)
	return Window{From: start, Until: start.AddDate(0, 0, 1)}
}

// DaysWindow is the n UTC days before the day t belongs to.
func DaysWindow(now time.Time, n int) Window {
	if n < 0 {
		n = 0
	}
	until := Day(now).AddDate(0, 0, 1)
	return Window{From: until.AddDate(0, 0, -n), Until: until}
}

// Contains reports whether t is in the window.
func (w Window) Contains(t time.Time) bool {
	u := t.UTC()
	return !u.Before(w.From) && u.Before(w.Until)
}

// Days is how many whole UTC days the window spans.
func (w Window) Days() int {
	if !w.Until.After(w.From) {
		return 0
	}
	return int(w.Until.Sub(w.From).Hours() / 24)
}

// Describe renders the window the way a page prints it: the two UTC days it covers.
func (w Window) Describe() string {
	if w.From.IsZero() && w.Until.IsZero() {
		return "no window"
	}
	last := w.Until.Add(-time.Second)
	return fmt.Sprintf("%s to %s (UTC)", w.From.Format("2006-01-02"), last.Format("2006-01-02"))
}

// Clamp refuses a window whose end is not after its start: a caller that built one backwards would
// otherwise read as an empty range and silently sweep nothing.
func (w Window) Check() error {
	if w.Until.Before(w.From) {
		return fmt.Errorf("clock: window ends %s before it starts %s", Stamp(w.Until), Stamp(w.From))
	}
	return nil
}

// Labels renders the first and last day of the window, which is what a filename carries.
func (w Window) Labels() (string, string) {
	return w.From.Format("20060102"), w.Until.Add(-time.Second).Format("20060102")
}
