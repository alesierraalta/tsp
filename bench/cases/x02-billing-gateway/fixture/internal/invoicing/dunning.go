package invoicing

import (
	"fmt"
	"time"

	"example.com/billing/internal/clock"
	"example.com/billing/internal/money"
	"example.com/billing/internal/store"
)

// Step is one reminder of the collection schedule: how many days after the due date it goes out and
// on which channel.
type Step struct {
	Day     int
	Channel string
	Note    string
}

// Validate checks the step.
func (s Step) Validate() error {
	if s.Day < 0 {
		return fmt.Errorf("invoicing: a reminder on day %d is before the due date", s.Day)
	}
	if s.Channel == "" {
		return fmt.Errorf("invoicing: a reminder on day %d has no channel", s.Day)
	}
	return nil
}

// Schedule returns the instants the reminders of a collection schedule go out for an invoice due on
// the day given. The schedule is ordered by day, so two reminders on one day go out in the order they
// were declared.
func Schedule(due time.Time, steps []Step) ([]time.Time, error) {
	out := make([]time.Time, 0, len(steps))
	for _, step := range steps {
		if err := step.Validate(); err != nil {
			return nil, err
		}
		out = append(out, clock.Day(due).AddDate(0, 0, step.Day))
	}
	return out, nil
}

// DefaultSchedule is the collection schedule a deployment starts with: a reminder on the due date, a
// second one a week later, and the past-due notice after that.
func DefaultSchedule() []Step {
	return []Step{
		{Day: 0, Channel: "email", Note: "the invoice is due today"},
		{Day: 7, Channel: "email", Note: "the invoice is a week overdue"},
		{Day: 14, Channel: "email", Note: "the invoice is two weeks overdue"},
		{Day: 21, Channel: "post", Note: "final notice before suspension"},
	}
}

// DueState is where an invoice stands against its due date.
type DueState string

// The states an invoice can be in against its due date.
const (
	// NotDue is an invoice whose due date has not arrived.
	NotDue DueState = "not_due"
	// Due is an invoice due today.
	Due DueState = "due"
	// Overdue is an invoice whose due date passed and that is not paid.
	Overdue DueState = "overdue"
	// Settled is an invoice that is paid or void.
	Settled DueState = "settled"
)

// State reports where an invoice stands at an instant.
func State(invoice store.Invoice, at time.Time) DueState {
	switch invoice.Status {
	case store.InvoicePaid, store.InvoiceVoid:
		return Settled
	case store.InvoiceDraft:
		return NotDue
	}
	due := clock.Day(invoice.Due)
	today := clock.Day(at)
	switch {
	case today.Before(due):
		return NotDue
	case today.Equal(due):
		return Due
	}
	return Overdue
}

// DaysOverdue reports how many UTC days past its due date an invoice is, zero when it is not overdue.
func DaysOverdue(invoice store.Invoice, at time.Time) int64 {
	if State(invoice, at) != Overdue {
		return 0
	}
	return clock.DaysBetween(invoice.Due, at)
}

// Reminders returns the steps that are due for an invoice at an instant, in schedule order.
func Reminders(invoice store.Invoice, at time.Time, steps []Step) ([]Step, error) {
	if State(invoice, at) != Overdue {
		return nil, nil
	}
	overdue := DaysOverdue(invoice, at)
	var out []Step
	for _, step := range steps {
		if err := step.Validate(); err != nil {
			return nil, err
		}
		if int64(step.Day) <= overdue {
			out = append(out, step)
		}
	}
	return out, nil
}

// Dunning is the state a collection run reports for one invoice.
type Dunning struct {
	Invoice     string
	State       DueState
	Days        int64
	Reminders   int
	Outstanding money.Amount
}

// Run returns the dunning state of every open invoice of a tenant at an instant.
func Run(invoices []store.Invoice, at time.Time, steps []Step) ([]Dunning, error) {
	out := make([]Dunning, 0, len(invoices))
	for _, invoice := range invoices {
		state := State(invoice, at)
		if state == Settled || state == NotDue {
			continue
		}
		remainder, err := Outstanding(invoice)
		if err != nil {
			return nil, err
		}
		reminders, err := Reminders(invoice, at, steps)
		if err != nil {
			return nil, err
		}
		out = append(out, Dunning{
			Invoice:     invoice.ID,
			State:       state,
			Days:        DaysOverdue(invoice, at),
			Reminders:   len(reminders),
			Outstanding: remainder,
		})
	}
	return out, nil
}

// PastDue returns the invoices a collection run would move to past due: issued, unpaid, and past
// their grace period.
func PastDue(invoices []store.Invoice, at time.Time, graceDays int64) []store.Invoice {
	if graceDays < 0 {
		graceDays = 0
	}
	var out []store.Invoice
	for _, invoice := range invoices {
		if invoice.Status != store.InvoiceOpen {
			continue
		}
		if DaysOverdue(invoice, at) > graceDays {
			out = append(out, invoice)
		}
	}
	return out
}
