// Package calendar decides which accounting month a close runs for. A period is a calendar month in
// UTC. Every decision here is taken on the UTC clock: the service runs in whatever zone its host is
// set to, and a close that rolled over at local midnight would bill the wrong month.
package calendar

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// ErrMalformed is returned for a string that is not a period.
var ErrMalformed = errors.New("calendar: not an accounting period")

// Period is one accounting month, "YYYY-MM". It is used as a directory name, so the type only
// carries a value Parse accepted.
type Period string

// periodRE is anchored at both ends on purpose: an unanchored check would accept "2024-05-31" and
// "x2024-05", and that string is joined onto the workspace to name the statement directory.
var periodRE = regexp.MustCompile(`^(\d{4})-(0[1-9]|1[0-2])$`)

// Parse reads a period. The value must be exactly seven bytes; surrounding space is refused rather
// than trimmed, because a padded period is a different key.
func Parse(raw string) (Period, error) {
	if !periodRE.MatchString(raw) {
		return "", fmt.Errorf("%w: %q", ErrMalformed, raw)
	}
	return Period(raw), nil
}

// Valid reports whether p is a period. The zero Period is not.
func (p Period) Valid() bool { return periodRE.MatchString(string(p)) }

// String is the wire and directory form.
func (p Period) String() string { return string(p) }

// Year is the four-digit year.
func (p Period) Year() int {
	n, _ := strconv.Atoi(string(p)[0:4])
	return n
}

// Month is the month number, 1 to 12.
func (p Period) Month() int {
	n, _ := strconv.Atoi(string(p)[5:7])
	return n
}

// Bounds returns the first instant of the month and the first instant of the next one, in UTC. The
// half-open range [start, end) is what event filtering and the window check compare against.
func (p Period) Bounds() (start, end time.Time) {
	if !p.Valid() {
		return time.Time{}, time.Time{}
	}
	start = time.Date(p.Year(), time.Month(p.Month()), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0)
}

// Contains reports whether an instant falls inside the period, by the UTC calendar.
func (p Period) Contains(t time.Time) bool {
	start, end := p.Bounds()
	if start.IsZero() {
		return false
	}
	return !t.Before(start) && t.Before(end)
}

// Of is the period an instant belongs to. The instant is converted to UTC first: a local
// "2024-05-01 00:30:00+02:00" is "2024-04-30 22:30:00Z" and belongs to April.
func Of(t time.Time) Period {
	utc := t.UTC()
	return Period(fmt.Sprintf("%04d-%02d", utc.Year(), int(utc.Month())))
}

// MonthsBetween reports how many months separate earlier from later. A negative result means later
// is the older period.
func MonthsBetween(earlier, later Period) int {
	if !earlier.Valid() || !later.Valid() {
		return 0
	}
	return (later.Year()*12 + later.Month()) - (earlier.Year()*12 + earlier.Month())
}
