package proration

import (
	"fmt"
	"time"

	"example.com/billing/internal/clock"
	"example.com/billing/internal/money"
)

// Pricing is the part of a plan a seat change is priced against.
type Pricing struct {
	// SeatPrice is what one seat costs for a whole cycle.
	SeatPrice money.Amount
	// IncludedSeats is how many seats the plan already carries, which the plan's base price covers.
	IncludedSeats int
}

// Validate checks the pricing.
func (p Pricing) Validate() error {
	if err := p.SeatPrice.Validate(); err != nil {
		return err
	}
	if _, err := p.SeatPrice.NonNegative(); err != nil {
		return err
	}
	if p.IncludedSeats < 0 {
		return fmt.Errorf("proration: a plan includes %d seats", p.IncludedSeats)
	}
	return nil
}

// Billable returns the number of seats a seat count pays for, which is the count above the plan's
// included seats.
func (p Pricing) Billable(seats int) int {
	if seats <= p.IncludedSeats {
		return 0
	}
	return seats - p.IncludedSeats
}

// SeatChange is one change to the seats of an active subscription.
type SeatChange struct {
	Pricing Pricing
	// From and To are the seat counts before and after the change.
	From int
	To   int
	// Period is the billing cycle the change was made in.
	Period clock.Period
	// At is the instant the change took effect.
	At time.Time
}

// Validate checks the change.
func (c SeatChange) Validate() error {
	if err := c.Pricing.Validate(); err != nil {
		return err
	}
	if c.From < 0 || c.To < 0 || c.From > 10000 || c.To > 10000 {
		return fmt.Errorf("proration: seat counts %d and %d", c.From, c.To)
	}
	if c.From == c.To {
		return fmt.Errorf("proration: the seat count does not change")
	}
	if c.Period.Days() <= 0 {
		return ErrEmptyCycle
	}
	if !c.Period.Contains(c.At) {
		return fmt.Errorf("%w: %s is not inside %s", ErrNoCycle, clock.Format(c.At), c.Period)
	}
	return nil
}

// Added returns the number of seats the change adds, negative when it removes them.
func (c SeatChange) Added() int { return c.To - c.From }

// Charge returns what adding seats owes for the rest of the cycle.
//
// The price of a seat is the plan's seat price per cycle, and the seats are added for the days that
// are left, so the charge is the seats' whole-cycle price scaled by the remaining part of the cycle.
// A change that adds seats on the first day of the cycle owes the whole price and one made on the
// last day owes a single day; a change that adds no billable seats (the plan's base price already
// covers them) owes nothing.
func (c SeatChange) Charge() (money.Amount, error) {
	if err := c.Validate(); err != nil {
		return money.Amount{}, err
	}
	added := c.Pricing.Billable(c.To) - c.Pricing.Billable(c.From)
	if added <= 0 {
		return money.Zero(c.Pricing.SeatPrice.Currency), nil
	}
	whole, err := c.Pricing.SeatPrice.Mul(int64(added))
	if err != nil {
		return money.Amount{}, err
	}
	remaining := c.Period.RemainingDays(c.At)
	total := c.Period.Days()
	charge, err := prorateInFloat(whole, remaining, total)
	if err != nil {
		return money.Amount{}, err
	}
	return money.New(charge, whole.Currency)
}

// prorateInFloat scales a whole-cycle amount to the days that are left.
func prorateInFloat(amount money.Amount, numer, denom int64) (int64, error) {
	if denom <= 0 {
		return 0, ErrEmptyCycle
	}
	return int64(float64(amount.Minor) * float64(numer) / float64(denom)), nil
}

// Credit returns what removing seats is worth for the rest of the cycle. It is the mirror image of
// Charge and it goes through the shared split, so the credit for removing a seat and the charge for
// adding it back agree to the minor unit.
func (c SeatChange) Credit() (money.Amount, error) {
	if err := c.Validate(); err != nil {
		return money.Amount{}, err
	}
	removed := c.Pricing.Billable(c.From) - c.Pricing.Billable(c.To)
	if removed <= 0 {
		return money.Zero(c.Pricing.SeatPrice.Currency), nil
	}
	whole, err := c.Pricing.SeatPrice.Mul(int64(removed))
	if err != nil {
		return money.Amount{}, err
	}
	return MidCycle(whole, c.Period, c.At)
}

// Line renders the change as the invoice line it becomes: the description an operator reads, the
// quantity and the amount.
func (c SeatChange) Line(amount money.Amount) (string, int64) {
	added := c.Added()
	switch {
	case added > 0:
		return fmt.Sprintf("Seats added (%d to %d), %s of the cycle",
			c.From, c.To, factorText(c.Period, c.At)), int64(added)
	case added < 0:
		return fmt.Sprintf("Seats removed (%d to %d), %s of the cycle",
			c.From, c.To, factorText(c.Period, c.At)), int64(-added)
	}
	return "No seat change", 0
}

// factorText renders the remaining part of a cycle for a description.
func factorText(period clock.Period, at time.Time) string {
	factor, err := Remaining(period, at)
	if err != nil {
		return "0 days"
	}
	return fmt.Sprintf("%d/%d", factor.Numer, factor.Denom)
}
