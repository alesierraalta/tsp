package proration

import (
	"fmt"
	"time"

	"example.com/billing/internal/clock"
	"example.com/billing/internal/money"
)

// Trial is the end of a trial and the price it converts to.
type Trial struct {
	// Period is the cycle the subscription converts into.
	Period clock.Period
	// Price is the whole-cycle price of the plan the trial converts to.
	Price money.Amount
	// EndsAt is the instant the trial ended, which is also the instant the cycle starts.
	EndsAt time.Time
	// ConvertedAt is the instant the conversion was recorded.
	ConvertedAt time.Time
}

// Charge returns what the rest of the conversion cycle costs. A trial that ends on the first day of
// the cycle owes the whole price; one that ends part way through owes the days that are left.
func (t Trial) Charge() (money.Amount, error) {
	if err := t.Price.Validate(); err != nil {
		return money.Amount{}, err
	}
	if _, err := t.Price.NonNegative(); err != nil {
		return money.Amount{}, err
	}
	if t.Period.Days() <= 0 {
		return money.Amount{}, ErrEmptyCycle
	}
	at := t.ConvertedAt
	if at.IsZero() {
		at = t.EndsAt
	}
	if !t.Period.Contains(at) {
		return money.Amount{}, fmt.Errorf("%w: the conversion at %s is not inside %s",
			ErrNoCycle, clock.Format(at), t.Period)
	}
	return MidCycle(t.Price, t.Period, at)
}

// AddOn is one extra the subscription carries beside its plan.
type AddOn struct {
	// Name is the add-on's label on the invoice.
	Name string
	// Price is the whole-cycle price of the add-on.
	Price money.Amount
	// StartsAt and EndsAt bound the days the add-on applies to.
	StartsAt time.Time
	EndsAt   time.Time
}

// Validate checks the add-on.
func (a AddOn) Validate() error {
	if a.Name == "" {
		return fmt.Errorf("proration: an add-on needs a name")
	}
	if err := a.Price.Validate(); err != nil {
		return err
	}
	if _, err := a.Price.NonNegative(); err != nil {
		return err
	}
	if a.StartsAt.IsZero() || a.EndsAt.IsZero() {
		return fmt.Errorf("proration: add-on %q needs both instants", a.Name)
	}
	if !a.EndsAt.After(a.StartsAt) {
		return fmt.Errorf("proration: add-on %q ends before it starts", a.Name)
	}
	return nil
}

// Charge prices the add-on over the part of the cycle it covers. The days are counted from the day
// the add-on starts to the end of the cycle, and a part of a day counts as a whole one: an add-on
// that starts at noon applies to that day.
func (a AddOn) Charge(period clock.Period) (money.Amount, error) {
	if err := a.Validate(); err != nil {
		return money.Amount{}, err
	}
	start := a.StartsAt
	if start.Before(period.Start) {
		start = period.Start
	}
	end := a.EndsAt
	if end.After(period.End) {
		end = period.End
	}
	if !end.After(start) {
		return money.Zero(a.Price.Currency), nil
	}
	total := period.Days()
	if total <= 0 {
		return money.Amount{}, ErrEmptyCycle
	}
	covered := clock.DaysBetween(clock.Day(start), end)
	if covered <= 0 {
		covered = 1
	}
	if covered > total {
		covered = total
	}
	return money.ProrateAmount(a.Price, covered, total)
}

// Retainer is a support retainer billed for the days a subscription held it.
type Retainer struct {
	Monthly money.Amount
	Period  clock.Period
	HeldAt  time.Time
}

// Charge prices the retainer for the days left in the cycle at HeldAt.
func (r Retainer) Charge() (money.Amount, error) {
	if err := r.Monthly.Validate(); err != nil {
		return money.Amount{}, err
	}
	return MidCycle(r.Monthly, r.Period, r.HeldAt)
}
