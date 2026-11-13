package invoicing

import (
	"fmt"

	"example.com/billing/internal/ids"
	"example.com/billing/internal/metering"
	"example.com/billing/internal/money"
	"example.com/billing/internal/store"
	"example.com/billing/internal/tax"
)

// LineKinds are the kinds of line an invoice carries. A kind is how a report groups the lines and how
// an operator recognizes a line without reading its description.
const (
	// KindBase is the plan's base price.
	KindBase = "base"
	// KindSeats is the seats above what the plan includes.
	KindSeats = "seats"
	// KindUsage is metered overage.
	KindUsage = "usage"
	// KindProration is a change made part way through the cycle.
	KindProration = "proration"
	// KindTax is the tax on the taxable base.
	KindTax = "tax"
	// KindCredit is a credit applied to the invoice.
	KindCredit = "credit"
)

// BaseLine is the plan's base price for one cycle.
func BaseLine(plan store.Plan, currency string) (store.Line, error) {
	if plan.BasePrice.Currency != currency {
		return store.Line{}, fmt.Errorf("invoicing: plan %q prices in %s and the invoice is in %s",
			plan.ID, plan.BasePrice.Currency, currency)
	}
	return store.Line{
		ID:          ids.New(ids.Line),
		Kind:        KindBase,
		Description: plan.Name + " base price",
		Quantity:    1,
		UnitPrice:   plan.BasePrice,
		Amount:      plan.BasePrice,
	}, nil
}

// SeatLine is the price of the seats the subscription carries above the plan's included count.
func SeatLine(plan store.Plan, seats int, currency string) (store.Line, error) {
	billable := seats - plan.IncludedSeats
	if billable <= 0 {
		return store.Line{
			ID:          ids.New(ids.Line),
			Kind:        KindSeats,
			Description: fmt.Sprintf("%s seats", plan.Name),
			Quantity:    0,
			UnitPrice:   money.Zero(currency),
			Amount:      money.Zero(currency),
		}, nil
	}
	amount, err := plan.SeatPrice.Mul(int64(billable))
	if err != nil {
		return store.Line{}, err
	}
	if plan.SeatPrice.Currency != currency {
		return store.Line{}, fmt.Errorf("invoicing: plan %q prices seats in %s and the invoice is in %s",
			plan.ID, plan.SeatPrice.Currency, currency)
	}
	return store.Line{
		ID:          ids.New(ids.Line),
		Kind:        KindSeats,
		Description: fmt.Sprintf("%s seats above the %d included", plan.Name, plan.IncludedSeats),
		Quantity:    int64(billable),
		UnitPrice:   plan.SeatPrice,
		Amount:      amount,
	}, nil
}

// UsageLines is one line per metric the plan prices, in metric order. A quantity inside the plan's
// included amount produces no line at all rather than a line of zero, so an invoice reads as what was
// billed.
func UsageLines(plan store.Plan, totals map[string]int64, currency string) ([]store.Line, error) {
	var lines []store.Line
	for _, metric := range metering.Metrics(totals) {
		if plan.Metric != "" && metric != plan.Metric {
			continue
		}
		units := metering.Overage(plan, totals[metric])
		if units <= 0 {
			continue
		}
		amount, err := metering.Charge(plan, units)
		if err != nil {
			return nil, err
		}
		if amount.Currency != currency {
			return nil, fmt.Errorf("invoicing: plan %q meters in %s and the invoice is in %s",
				plan.ID, amount.Currency, currency)
		}
		lines = append(lines, store.Line{
			ID:          ids.New(ids.Line),
			Kind:        KindUsage,
			Description: fmt.Sprintf("%s overage beyond %d included", metric, plan.IncludedQuantity),
			Quantity:    units,
			UnitPrice:   plan.UnitPrice,
			Amount:      amount,
		})
	}
	return lines, nil
}

// ProrationLine is a line a change made part way through the cycle produced.
func ProrationLine(description string, quantity int64, unitPrice, amount money.Amount, currency string) (store.Line, error) {
	if amount.Currency != currency || unitPrice.Currency != currency {
		return store.Line{}, fmt.Errorf("invoicing: a proration line in %s on an invoice in %s",
			amount.Currency, currency)
	}
	return store.Line{
		ID:          ids.New(ids.Line),
		Kind:        KindProration,
		Description: description,
		Quantity:    quantity,
		UnitPrice:   unitPrice,
		Amount:      amount,
	}, nil
}

// TaxLine is the tax on the taxable base.
func TaxLine(rate tax.Rate, base, amount money.Amount, currency string) (store.Line, error) {
	if amount.Currency != currency {
		return store.Line{}, fmt.Errorf("invoicing: tax in %s on an invoice in %s", amount.Currency, currency)
	}
	return store.Line{
		ID:          ids.New(ids.Line),
		Kind:        KindTax,
		Description: fmt.Sprintf("Tax %s on %s", rate, base),
		Quantity:    1,
		UnitPrice:   amount,
		Amount:      amount,
	}, nil
}

// CreditLine is the credit an account carries, written as a negative line. A zero credit produces no
// line, which is what keeps a paid invoice from carrying an empty credit.
func CreditLine(credits money.Amount, currency string) (*store.Line, error) {
	if credits.Minor == 0 {
		return nil, nil
	}
	if credits.Currency != currency {
		return nil, fmt.Errorf("invoicing: a credit in %s on an invoice in %s", credits.Currency, currency)
	}
	amount, err := credits.Neg()
	if err != nil {
		return nil, err
	}
	return &store.Line{
		ID:          ids.New(ids.Line),
		Kind:        KindCredit,
		Description: "Credit applied",
		Quantity:    1,
		UnitPrice:   amount,
		Amount:      amount,
	}, nil
}

// KindTotals sums the lines of an invoice by kind, which is what a revenue report breaks a total into.
func KindTotals(lines []store.Line) (map[string]money.Amount, error) {
	totals := make(map[string]money.Amount, 6)
	for _, line := range lines {
		current, ok := totals[line.Kind]
		if !ok {
			totals[line.Kind] = line.Amount
			continue
		}
		next, err := current.Add(line.Amount)
		if err != nil {
			return nil, err
		}
		totals[line.Kind] = next
	}
	return totals, nil
}
