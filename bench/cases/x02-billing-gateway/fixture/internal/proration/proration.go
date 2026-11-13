// Package proration prices the part of a billing cycle a change covers. A change made on the ninth
// day of a thirty-day cycle is billed for the twenty-one days it applies to, and the part is computed
// from whole UTC days so the answer does not depend on the hour a request happened to arrive.
package proration

import (
	"errors"
	"fmt"
	"time"

	"example.com/billing/internal/clock"
	"example.com/billing/internal/money"
)

// Errors a proration reports.
var (
	// ErrNoCycle is returned for a change made outside the cycle it belongs to.
	ErrNoCycle = errors.New("proration: the change is outside the cycle")
	// ErrEmptyCycle is returned for a cycle with no days to prorate over.
	ErrEmptyCycle = errors.New("proration: the cycle covers no days")
)

// Factor is the part of a cycle a change covers, as a pair of day counts: Numer of Denom days.
type Factor struct {
	Numer int64
	Denom int64
}

// Remaining returns the factor of the days of a cycle that are left at at.
func Remaining(period clock.Period, at time.Time) (Factor, error) {
	denom := period.Days()
	if denom <= 0 {
		return Factor{}, ErrEmptyCycle
	}
	if !period.Contains(at) {
		return Factor{}, fmt.Errorf("%w: %s is not inside %s", ErrNoCycle, clock.Format(at), period)
	}
	numer := period.RemainingDays(at)
	if numer <= 0 {
		return Factor{}, fmt.Errorf("%w: no days are left at %s", ErrNoCycle, clock.Format(at))
	}
	if numer > denom {
		numer = denom
	}
	return Factor{Numer: numer, Denom: denom}, nil
}

// Elapsed returns the factor of the days of a cycle that had started at at.
func Elapsed(period clock.Period, at time.Time) (Factor, error) {
	denom := period.Days()
	if denom <= 0 {
		return Factor{}, ErrEmptyCycle
	}
	numer := period.ElapsedDays(at)
	if numer < 0 {
		numer = 0
	}
	if numer > denom {
		numer = denom
	}
	return Factor{Numer: numer, Denom: denom}, nil
}

// Whole reports whether the factor covers the whole cycle.
func (f Factor) Whole() bool { return f.Numer == f.Denom && f.Denom > 0 }

// Of returns the part of an amount the factor covers. The arithmetic is the shared minor-unit split:
// the factor is two day counts, the product stays in integers and a half minor unit rounds up, so a
// charge and the credit that undoes it agree to the cent.
func (f Factor) Of(amount money.Amount) (money.Amount, error) {
	if f.Denom <= 0 {
		return money.Amount{}, ErrEmptyCycle
	}
	if f.Numer < 0 || f.Numer > f.Denom {
		return money.Amount{}, fmt.Errorf("proration: the factor covers %d of %d days", f.Numer, f.Denom)
	}
	if f.Whole() {
		return amount, nil
	}
	return money.ProrateAmount(amount, f.Numer, f.Denom)
}

// String renders the factor the way a log line and an invoice note carry it.
func (f Factor) String() string {
	return fmt.Sprintf("%d/%d", f.Numer, f.Denom)
}

// MidCycle returns the part of a cycle's price that is left at at. Every proration path in the
// service is this function or a factor of it: the seat credit, the trial conversion charge, the add-on
// charge and the support retainer all price a partial cycle, and pricing one of them another way is
// how two lines of the same invoice come to disagree about the same days.
func MidCycle(total money.Amount, period clock.Period, at time.Time) (money.Amount, error) {
	factor, err := Remaining(period, at)
	if err != nil {
		return money.Amount{}, err
	}
	return factor.Of(total)
}

// ElapsedPart returns the part of a cycle's price that had been consumed at at. It is MidCycle's
// mirror image, used where a change is priced from the days already served.
func ElapsedPart(total money.Amount, period clock.Period, at time.Time) (money.Amount, error) {
	factor, err := Elapsed(period, at)
	if err != nil {
		return money.Amount{}, err
	}
	return factor.Of(total)
}

// Sum adds the parts of one currency, which is how the lines of a prorated invoice are totalled.
func Sum(parts ...money.Amount) (money.Amount, error) {
	if len(parts) == 0 {
		return money.Amount{}, fmt.Errorf("proration: sum needs at least one amount")
	}
	return money.Sum(parts...)
}
