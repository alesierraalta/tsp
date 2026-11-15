package proration

import (
	"fmt"
	"time"

	"example.com/billing/internal/clock"
	"example.com/billing/internal/money"
)

// CreditNote is a credit an account is owed for a change made inside a cycle.
type CreditNote struct {
	// Reason is what the customer is told the credit is for.
	Reason string
	// Amount is what the credit is worth.
	Amount money.Amount
}

// Validate checks the note.
func (n CreditNote) Validate() error {
	if n.Reason == "" {
		return fmt.Errorf("proration: a credit note needs a reason")
	}
	if err := n.Amount.Validate(); err != nil {
		return err
	}
	if _, err := n.Amount.NonNegative(); err != nil {
		return err
	}
	return nil
}

// Notes returns the credit notes a cancelled subscription is owed for the days it will not use. A
// cancellation at instant at leaves the days from at to the end of the cycle unbilled, and the credit
// is exactly the part of the cycle's price those days carry, priced by the shared split.
func Notes(subtotal money.Amount, period clock.Period, at time.Time) ([]CreditNote, error) {
	if err := subtotal.Validate(); err != nil {
		return nil, err
	}
	amount, err := MidCycle(subtotal, period, at)
	if err != nil {
		return nil, err
	}
	if amount.Minor == 0 {
		return nil, nil
	}
	note := CreditNote{
		Reason: fmt.Sprintf("Cancellation on %s: %s of the cycle not served", factorString(period, at), "the remaining days"),
		Amount: amount,
	}
	if err := note.Validate(); err != nil {
		return nil, err
	}
	return []CreditNote{note}, nil
}

// SeatCreditNote is the credit for seats removed part way through a cycle.
func SeatCreditNote(change SeatChange) (CreditNote, error) {
	amount, err := change.Credit()
	if err != nil {
		return CreditNote{}, err
	}
	description, _ := change.Line(amount)
	return CreditNote{Reason: description, Amount: amount}, nil
}

// factorString renders the part of the cycle a note covers.
func factorString(period clock.Period, at time.Time) string {
	factor, err := Remaining(period, at)
	if err != nil {
		return "0 days"
	}
	return factor.String()
}
