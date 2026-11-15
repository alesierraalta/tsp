// Package clock is the service's only source of time. Retention windows, batch ages and audit
// stamps read it, so a test can pin every instant the service observes.
package clock

import (
	"fmt"
	"time"
)

// Clock returns the instant the service should treat as now.
type Clock func() time.Time

// Fixed is a clock that always reports t. A test uses it so a retention sweep is decided by the
// case's own timestamps and not by the wall clock.
func Fixed(t time.Time) Clock { return func() time.Time { return t } }

// System is the wall clock in UTC. The service never reports a local instant.
func System() Clock { return func() time.Time { return time.Now().UTC() } }

// Now reads a clock that may be nil, so a caller that forgot to set one still reports an instant.
func Now(c Clock) time.Time {
	if c == nil {
		return time.Now().UTC()
	}
	return c().UTC()
}

// Day truncates t to the UTC day it belongs to. Every retention decision compares days, never
// instants, because a batch that ends at 23:59 belongs to the day it started on.
func Day(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// AddDays returns the UTC day n days from t, which is how a retention horizon is written.
func AddDays(t time.Time, n int) time.Time { return Day(t).AddDate(0, 0, n) }

// DaysBetween reports how many whole days separate two instants, counted on UTC days. A negative
// number means the second instant is earlier.
func DaysBetween(from, to time.Time) int {
	a, b := Day(from), Day(to)
	return int(b.Sub(a).Hours() / 24)
}

// ParseRFC3339 reads an instant a manifest declares. An empty value is refused rather than read as
// the zero time, because a batch with no creation instant cannot be retained.
func ParseRFC3339(s string) (time.Time, error) {
	trimmed := s
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("clock: empty timestamp")
	}
	t, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return time.Time{}, fmt.Errorf("clock: %q is not RFC3339: %w", s, err)
	}
	return t.UTC(), nil
}

// LoadZone reads an IANA zone name. The service keeps one configured zone for reporting only; every
// stored instant and every retention comparison stays in UTC.
func LoadZone(name string) (*time.Location, error) {
	if name == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("clock: zone %q: %w", name, err)
	}
	return loc, nil
}

// Stamp renders an instant the way the audit trail and the HTML pages write it: UTC, RFC3339,
// second precision.
func Stamp(t time.Time) string { return t.UTC().Truncate(time.Second).Format(time.RFC3339) }
