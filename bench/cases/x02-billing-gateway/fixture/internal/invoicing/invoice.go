// Package invoicing builds the bill for one subscription and one cycle: the plan's base price, the
// seats above what the plan includes, the metered overage, the tax on the taxable base, and the
// credits an account already carries. Every amount is money.Amount over a total that is the sum of
// the lines, so an invoice whose lines do not add up is refused rather than stored.
package invoicing

import (
	"fmt"
	"time"

	"example.com/billing/internal/clock"
	"example.com/billing/internal/ids"
	"example.com/billing/internal/metering"
	"example.com/billing/internal/money"
	"example.com/billing/internal/store"
	"example.com/billing/internal/tax"
)

// Assembly is everything an invoice is built from. A caller that has no prorated charge of its own
// leaves Extra empty; the base, seat, usage and tax lines are always computed here.
type Assembly struct {
	Tenant       string
	Account      string
	Subscription string
	Plan         store.Plan
	Seats        int
	Period       clock.Period
	Issued       time.Time
	Due          time.Time
	Currency     string
	// Usage is the metered quantity per metric inside the period.
	Usage map[string]int64
	// TaxRate is the rate the account's region charges.
	TaxRate tax.Rate
	// TaxBasis is the denominator the rate is expressed in.
	TaxBasis int
	// Credits is the balance the account carries, applied to the invoice.
	Credits money.Amount
	// Extra are lines a change made during the cycle produced, in the order they are printed.
	Extra []store.Line
}

// Assemble builds the invoice.
func Assemble(a Assembly) (store.Invoice, error) {
	if err := a.validate(); err != nil {
		return store.Invoice{}, err
	}
	lines, err := a.lines()
	if err != nil {
		return store.Invoice{}, err
	}
	total, err := Total(lines)
	if err != nil {
		return store.Invoice{}, err
	}
	paid, err := money.New(0, a.Currency)
	if err != nil {
		return store.Invoice{}, err
	}
	invoice := store.Invoice{
		ID:           ids.New(ids.Invoice),
		Tenant:       a.Tenant,
		Account:      a.Account,
		Subscription: a.Subscription,
		Period:       a.Period,
		Issued:       a.Issued,
		Due:          a.Due,
		Status:       store.InvoiceDraft,
		Currency:     a.Currency,
		Lines:        lines,
		Total:        total,
		Paid:         paid,
	}
	if err := invoice.Validate(); err != nil {
		return store.Invoice{}, err
	}
	return invoice, nil
}

// validate checks the assembly before a line is built, so a refusal names the field that is missing
// rather than the invoice that came out wrong.
func (a Assembly) validate() error {
	if a.Tenant == "" || a.Account == "" || a.Subscription == "" {
		return fmt.Errorf("invoicing: the assembly names tenant %q, account %q and subscription %q",
			a.Tenant, a.Account, a.Subscription)
	}
	if err := a.Plan.Validate(); err != nil {
		return err
	}
	if a.Plan.Tenant != a.Tenant {
		return fmt.Errorf("invoicing: plan %q belongs to tenant %q", a.Plan.ID, a.Plan.Tenant)
	}
	if a.Period.Days() <= 0 {
		return fmt.Errorf("invoicing: the period %s covers no days", a.Period)
	}
	if !a.Due.After(a.Issued) {
		return fmt.Errorf("invoicing: the invoice is due %s and issued %s",
			clock.Format(a.Due), clock.Format(a.Issued))
	}
	if a.Seats < 0 {
		return fmt.Errorf("invoicing: the subscription carries %d seats", a.Seats)
	}
	if a.TaxBasis <= 0 {
		return fmt.Errorf("invoicing: the tax basis is %d", a.TaxBasis)
	}
	if err := a.TaxRate.Validate(a.TaxBasis); err != nil {
		return err
	}
	return nil
}

// lines builds the invoice's lines in the order the invoice prints them.
func (a Assembly) lines() ([]store.Line, error) {
	var lines []store.Line

	base, err := BaseLine(a.Plan, a.Currency)
	if err != nil {
		return nil, err
	}
	if base.Amount.Minor != 0 {
		lines = append(lines, base)
	}
	seats, err := SeatLine(a.Plan, a.Seats, a.Currency)
	if err != nil {
		return nil, err
	}
	if seats.Amount.Minor != 0 {
		lines = append(lines, seats)
	}
	usageLines, err := UsageLines(a.Plan, a.Usage, a.Currency)
	if err != nil {
		return nil, err
	}
	lines = append(lines, usageLines...)
	lines = append(lines, a.Extra...)

	taxLine, err := a.taxLine(lines)
	if err != nil {
		return nil, err
	}
	if taxLine.Amount.Minor != 0 {
		lines = append(lines, taxLine)
	}
	credit, err := CreditLine(a.Credits, a.Currency)
	if err != nil {
		return nil, err
	}
	if credit != nil {
		lines = append(lines, *credit)
	}
	return lines, nil
}

// taxLine prices the tax on the taxable base, which is every line above it: a proration charge is
// taxed like the plan it belongs to, and a credit is applied after the tax so the customer is not
// taxed on money they are not paying.
func (a Assembly) taxLine(lines []store.Line) (store.Line, error) {
	base, err := Total(lines)
	if err != nil {
		return store.Line{}, err
	}
	amount, err := a.TaxRate.Of(a.TaxBasis, base)
	if err != nil {
		return store.Line{}, err
	}
	return TaxLine(a.TaxRate, base, amount, a.Currency)
}

// UsageUnits is the billable quantity of every metric the plan prices, in metric order, which the
// usage report and the invoice both read.
func (a Assembly) UsageUnits() map[string]int64 {
	units := make(map[string]int64, len(a.Usage))
	for metric, quantity := range a.Usage {
		if a.Plan.Metric != "" && metric != a.Plan.Metric {
			continue
		}
		units[metric] = metering.Overage(a.Plan, quantity)
	}
	return units
}

// BilledQuantity returns the quantity of the plan's own metric an invoice bills, for the summary
// line an operator reads.
func BilledQuantity(plan store.Plan, totals map[string]int64) int64 {
	if plan.Metric == "" {
		return 0
	}
	return metering.Overage(plan, totals[plan.Metric])
}
