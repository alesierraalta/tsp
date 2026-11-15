package clock

import "time"

// Period is a half-open window of instants: Start is inside it and End is not.
type Period struct {
	Start time.Time
	End   time.Time
}

// New returns the period between two instants. A period whose end is not after its start is refused,
// because a zero-length cycle divides every proration factor by it.
func New(start, end time.Time) (Period, error) {
	start, end = start.UTC(), end.UTC()
	if !end.After(start) {
		return Period{}, ErrEmptyPeriod
	}
	return Period{Start: start, End: end}, nil
}

// Days is the number of whole UTC days the period covers. A cycle that starts at midnight and ends a
// month later covers the days in between, which is the denominator of every proration.
func (p Period) Days() int64 {
	return DaysBetween(p.Start, p.End)
}

// Contains reports whether t falls inside the period.
func (p Period) Contains(t time.Time) bool {
	t = t.UTC()
	return !t.Before(p.Start) && t.Before(p.End)
}

// Clamp returns the part of the period that starts at t. An instant before the start leaves the
// period untouched, and an instant after the end yields an empty remainder.
func (p Period) Clamp(t time.Time) Period {
	t = t.UTC()
	if t.Before(p.Start) {
		return p
	}
	if !t.Before(p.End) {
		return Period{Start: p.End, End: p.End}
	}
	return Period{Start: t, End: p.End}
}

// RemainingDays reports how many whole UTC days of the period are left at t. A day counts once it has
// started, so a change made at 23:50 still counts the day it lands on.
func (p Period) RemainingDays(t time.Time) int64 {
	rest := p.Clamp(t)
	if !rest.End.After(rest.Start) {
		return 0
	}
	return rest.Days()
}

// ElapsedDays reports how many whole UTC days of the period had started before t.
func (p Period) ElapsedDays(t time.Time) int64 {
	if !t.After(p.Start) {
		return 0
	}
	if !t.Before(p.End) {
		return p.Days()
	}
	return DaysBetween(p.Start, t)
}

// String renders the period the way a log line and an invoice header carry it.
func (p Period) String() string {
	return p.Start.Format(time.DateOnly) + ".." + p.End.Format(time.DateOnly)
}

// AddMonths adds months to t, clamping the day to the last day of the resulting month. A subscription
// anchored on the 31st renews on the 30th of a month that has thirty days and returns to the 31st
// afterwards, because the anchor is the day the customer signed up, not the day of the last invoice.
func AddMonths(t time.Time, months int) time.Time {
	t = t.UTC()
	year, month, day := t.Date()
	if months != 0 {
		total := int(month) - 1 + months
		year += floorDiv(total, 12)
		month = time.Month(floorMod(total, 12) + 1)
	}
	last := daysIn(year, month)
	if day > last {
		day = last
	}
	return time.Date(year, month, day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
}

// Cycle returns the period of one billing cycle that starts at start and runs for the given number of
// months. Both ends sit on a UTC midnight so the day count is exact.
func Cycle(start time.Time, months int) (Period, error) {
	if months <= 0 {
		return Period{}, ErrCycleLength
	}
	begin := Day(start)
	return New(begin, Day(AddMonths(begin, months)))
}

// MonthlyCycle returns the cycle of one month that contains the day anchor sits on, and the anchor of
// the next cycle. A subscription stores its anchor once; every cycle boundary is derived from it.
func MonthlyCycle(anchor, at time.Time) (Period, error) {
	day := Day(anchor).Day()
	last := daysIn(at.Year(), at.Month())
	if day > last {
		day = last
	}
	begin := time.Date(at.UTC().Year(), at.UTC().Month(), day, 0, 0, 0, 0, time.UTC)
	if at.UTC().Before(begin) {
		begin = AddMonths(begin, -1)
	}
	return Cycle(begin, 1)
}

// daysIn reports the number of days in a month.
func daysIn(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// floorDiv divides rounding towards negative infinity, which is what a month index below January of
// the anchor year needs.
func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// floorMod is the remainder that goes with floorDiv.
func floorMod(a, b int) int {
	return a - floorDiv(a, b)*b
}
