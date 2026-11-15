package clock

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrInvalidPeriod is reported for a billing period the service cannot read.
var ErrInvalidPeriod = errors.New("clock: invalid period")

// Period is one accounting month. It carries a year and a month and nothing else: a period is a
// calendar label, not an interval, so it has no zone and two deployments that bill the same month
// bill the same period.
type Period struct {
	Year  int
	Month time.Month
}

// ParsePeriod reads "YYYY-MM". The form is anchored: a four-digit year from 2000 to 2999, a dash, and
// a two-digit month from 01 to 12. Anything else — "2024-5", "2024-13", "2024-05-01", a value with a
// space — is refused instead of read as the nearest month.
func ParsePeriod(raw string) (Period, error) {
	if len(raw) != 7 || raw[4] != '-' {
		return Period{}, fmt.Errorf("%w: %q", ErrInvalidPeriod, raw)
	}
	year, err := strconv.Atoi(raw[:4])
	if err != nil || year < 2000 || year > 2999 {
		return Period{}, fmt.Errorf("%w: %q", ErrInvalidPeriod, raw)
	}
	month, err := strconv.Atoi(raw[5:])
	if err != nil || month < 1 || month > 12 {
		return Period{}, fmt.Errorf("%w: %q", ErrInvalidPeriod, raw)
	}
	if raw[5] < '0' || raw[5] > '9' || raw[6] < '0' || raw[6] > '9' {
		return Period{}, fmt.Errorf("%w: %q", ErrInvalidPeriod, raw)
	}
	return Period{Year: year, Month: time.Month(month)}, nil
}

// PeriodOf is the accounting month an instant belongs to. The instant is converted to UTC first, so
// a scan at 23:30 in a zone east of UTC is still in the month the sender's clock says and a close
// run anywhere agrees; the deployment's reporting zone is a display choice, never a period boundary.
func PeriodOf(t time.Time) Period {
	utc := t.UTC()
	return Period{Year: utc.Year(), Month: utc.Month()}
}

// String renders the period in the form ParsePeriod reads.
func (p Period) String() string {
	return fmt.Sprintf("%04d-%02d", p.Year, int(p.Month))
}

// Valid reports whether the period names a real month.
func (p Period) Valid() bool {
	return p.Year >= 2000 && p.Year <= 2999 && p.Month >= 1 && p.Month <= 12
}

// Start is the first instant of the period in UTC.
func (p Period) Start() time.Time {
	return time.Date(p.Year, p.Month, 1, 0, 0, 0, 0, time.UTC)
}

// End is the first instant of the next period in UTC, which is the exclusive end of this one.
func (p Period) End() time.Time { return p.Next().Start() }

// Contains reports whether an instant falls inside the period. The end is exclusive, so two
// adjacent periods never both hold the same instant.
func (p Period) Contains(t time.Time) bool {
	return !t.Before(p.Start()) && t.Before(p.End())
}

// Next is the period after p.
func (p Period) Next() Period {
	if p.Month == time.December {
		return Period{Year: p.Year + 1, Month: time.January}
	}
	return Period{Year: p.Year, Month: p.Month + 1}
}

// Prev is the period before p.
func (p Period) Prev() Period {
	if p.Month == time.January {
		return Period{Year: p.Year - 1, Month: time.December}
	}
	return Period{Year: p.Year, Month: p.Month - 1}
}

// Index is the period's position on a month axis, for comparing and bounding.
func (p Period) Index() int { return p.Year*12 + int(p.Month) - 1 }

// Before reports whether p is earlier than q.
func (p Period) Before(q Period) bool { return p.Index() < q.Index() }

// Distance is how many months q is after p, negative when q is earlier.
func (p Period) Distance(q Period) int { return q.Index() - p.Index() }

// Finished reports whether the period is over by now: its last instant is before now.
func (p Period) Finished(now time.Time) bool { return p.End().Before(now.UTC()) }

// Closed reports whether the period is over by now, in the terms a close uses: the period's end is
// at or before the current instant, so a close run exactly at the boundary sees a finished month.
func (p Period) Closed(now time.Time) bool { return !p.End().After(now.UTC()) }

// WithinWindow reports whether p is no more than months back from the period now falls in. A period
// still running, or one further back than the window, is outside.
func WithinWindow(p Period, now time.Time, months int) bool {
	if months < 0 {
		return false
	}
	// Distance answers "how many months after the receiver is the argument", so the receiver is the
	// current period: a period one month back answers 1 and one month ahead answers -1.
	back := p.Distance(PeriodOf(now))
	return back >= 0 && back <= months
}

// FormatPeriods renders a list of periods, comma separated, for a header.
func FormatPeriods(periods []Period) string {
	parts := make([]string, 0, len(periods))
	for _, p := range periods {
		parts = append(parts, p.String())
	}
	return strings.Join(parts, ",")
}
