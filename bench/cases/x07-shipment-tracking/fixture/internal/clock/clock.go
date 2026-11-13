// Package clock is the service's only source of time. An instant is stored and compared in UTC, and
// the one place a wall clock decides something — the day a shipment is due, which the deployment
// pins to a time zone — is anchored here: the day is computed in that zone and converted back to UTC
// before anything is stored, so a cutoff is the same instant wherever the service is read.
package clock

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidInstant is reported for an instant the service cannot read or place.
var ErrInvalidInstant = errors.New("clock: invalid instant")

// Clock is a clock a test can replace. The zero value is not usable; New and System build one.
type Clock struct {
	now func() time.Time
}

// System returns the system clock.
func System() *Clock { return &Clock{now: time.Now} }

// New returns a clock whose Now is now.
func New(now func() time.Time) *Clock {
	if now == nil {
		now = time.Now
	}
	return &Clock{now: now}
}

// Now is the current instant in UTC.
func (c *Clock) Now() time.Time { return c.now().UTC() }

// SetNow replaces the function a clock reads, for a test that moves time forward.
func (c *Clock) SetNow(now func() time.Time) {
	if now != nil {
		c.now = now
	}
}

// ParseInstant reads an RFC 3339 instant that carries an offset. A value without an offset is
// refused rather than read as UTC, because a sender that omits the offset does not know which day
// its scan belongs to.
func ParseInstant(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, fmt.Errorf("%w: empty", ErrInvalidInstant)
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q", ErrInvalidInstant, raw)
	}
	return parsed.UTC(), nil
}

// FormatInstant writes an instant in the form ParseInstant reads.
func FormatInstant(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// FormatDay writes the calendar day of an instant in the given zone.
func FormatDay(t time.Time, zone *time.Location) string {
	if zone == nil {
		zone = time.UTC
	}
	return t.In(zone).Format("2006-01-02")
}

// DayStart returns the first instant of the calendar day t falls on in zone, as a UTC instant. The
// day is not "24 hours before the next one": a zone that changes offset has days of 23 and 25 hours,
// and midnight is an instant the zone names, so it is built from the zone and converted.
func DayStart(t time.Time, zone *time.Location) time.Time {
	if zone == nil {
		zone = time.UTC
	}
	local := t.In(zone)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone).UTC()
}

// DayEnd returns the first instant of the next calendar day in zone, as a UTC instant.
func DayEnd(t time.Time, zone *time.Location) time.Time {
	return DayStart(DayStart(t, zone).Add(36*time.Hour), zone)
}

// DueBefore reports whether an instant is at or before the cutoff a shipment is due by. Both sides
// are compared as instants, so two zones naming the same moment agree.
func DueBefore(t, cutoff time.Time) bool { return !t.After(cutoff) }

// Age reports how old an instant is, negative for an instant in the future.
func Age(t, now time.Time) time.Duration { return now.Sub(t) }

// Within reports whether t sits in [now-maxAge, now+maxFuture]. An instant older than maxAge or
// further in the future than maxFuture is outside; the bounds are inclusive.
func Within(t, now time.Time, maxAge, maxFuture time.Duration) bool {
	if t.After(now.Add(maxFuture)) {
		return false
	}
	return !t.Before(now.Add(-maxAge))
}
