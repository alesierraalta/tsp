// Package clock is the billing service's only source of time. A deployment runs on the system clock;
// a test pins one instant and every cycle, invoice and report reads the same day.
package clock

import "time"

// Clock returns the instant a caller treats as now. It is a function so a test can replace it without
// the code under test knowing time exists.
type Clock func() time.Time

// System is the wall clock, read in UTC. A billing service that read local time would shift a cycle
// boundary twice a year and charge a customer for a day it never had.
func System() Clock {
	return func() time.Time { return time.Now().UTC() }
}

// Fixed returns a clock that always reports t, converted to UTC.
func Fixed(t time.Time) Clock {
	at := t.UTC()
	return func() time.Time { return at }
}

// Today returns the UTC day a clock reports: the instant truncated to midnight.
func Today(now Clock) time.Time {
	return Day(now())
}

// Day truncates t to the midnight of its UTC day. It is the granularity a retention window, a cycle
// boundary and an invoice day are compared at.
func Day(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// DayNumber reports the UTC day t falls on as a count of days since the Unix epoch. Two instants on
// the same day have the same number, and subtracting two numbers is the number of days between them,
// which is the arithmetic every proration factor is built from.
func DayNumber(t time.Time) int64 {
	return Day(t).Unix() / 86400
}

// DaysBetween reports how many whole UTC days separate from and to, signed.
func DaysBetween(from, to time.Time) int64 {
	return DayNumber(to) - DayNumber(from)
}

// Format renders an instant the way an API response and the trail carry it.
func Format(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// Parse reads an instant an operator or a client wrote. An instant without a zone is read as UTC:
// the service has one zone and a value that does not say which one is not allowed to guess.
func Parse(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}
