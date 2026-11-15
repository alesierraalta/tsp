// Package metering turns the usage events a customer reported into the quantities an invoice and a
// report are built from. It carries no clock and no store: a caller hands it the events of a period
// and gets the totals back, which is what makes the arithmetic testable on its own.
package metering

import (
	"fmt"
	"sort"

	"example.com/billing/internal/clock"
	"example.com/billing/internal/money"
	"example.com/billing/internal/store"
)

// Totals sums the quantities of a set of events by metric. Events of one metric add up whatever
// their status: a total is what the customer used, and the status is what has been billed, which is
// a different question answered by the invoice.
func Totals(events []store.UsageEvent) map[string]int64 {
	totals := make(map[string]int64)
	for _, event := range events {
		totals[event.Metric] += event.Quantity
	}
	return totals
}

// TotalsIn sums the events of one subscription inside a period.
func TotalsIn(events []store.UsageEvent, subscription string, period clock.Period) map[string]int64 {
	selected := make([]store.UsageEvent, 0, len(events))
	for _, event := range events {
		if event.Subscription != subscription {
			continue
		}
		if period.Contains(event.At) {
			selected = append(selected, event)
		}
	}
	return Totals(selected)
}

// Metrics lists the metrics of a total in order, so a rendered invoice is the same on every run.
func Metrics(totals map[string]int64) []string {
	names := make([]string, 0, len(totals))
	for name := range totals {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Overage returns the quantity above what a plan includes. A plan with no included quantity charges
// for every unit; a customer below the included quantity owes nothing extra.
func Overage(plan store.Plan, used int64) int64 {
	if plan.IncludedQuantity < 0 {
		return 0
	}
	if plan.IncludedQuantity == 0 {
		return used
	}
	if used <= plan.IncludedQuantity {
		return 0
	}
	return used - plan.IncludedQuantity
}

// Charge prices a quantity of billable units at the plan's unit price.
func Charge(plan store.Plan, units int64) (money.Amount, error) {
	if units < 0 {
		return money.Amount{}, fmt.Errorf("metering: %d units is not a quantity", units)
	}
	if units == 0 || plan.UnitPrice.Minor == 0 {
		return money.Zero(plan.UnitPrice.Currency), nil
	}
	return plan.UnitPrice.Mul(units)
}

// ChargeTotals prices the overage of every metric a plan bills, in metric order.
func ChargeTotals(plan store.Plan, totals map[string]int64) ([]money.Amount, error) {
	charges := make([]money.Amount, 0, len(totals))
	for _, metric := range Metrics(totals) {
		if plan.Metric != "" && metric != plan.Metric {
			// A metric the plan does not price is metered and reported but not billed, which is
			// what lets a customer report a new metric before the price list carries it.
			continue
		}
		units := Overage(plan, totals[metric])
		charge, err := Charge(plan, units)
		if err != nil {
			return nil, err
		}
		charges = append(charges, charge)
	}
	return charges, nil
}

// Report is the summary a usage report and the operator console read: what each subscription used,
// what was billed and what is still open.
type Report struct {
	Tenant       string
	Subscription string
	Account      string
	Metric       string
	Quantity     int64
	Billed       int64
	Recorded     int64
	Archived     int64
}

// Build returns one Report row per subscription and metric of the events given, in subscription and
// metric order.
func Build(tenant string, events []store.UsageEvent) []Report {
	type key struct{ subscription, metric string }
	rows := make(map[key]*Report)
	order := make([]key, 0, len(events))
	for _, event := range events {
		if event.Tenant != tenant {
			continue
		}
		k := key{subscription: event.Subscription, metric: event.Metric}
		row, ok := rows[k]
		if !ok {
			row = &Report{Tenant: tenant, Subscription: k.subscription, Account: event.Account, Metric: k.metric}
			rows[k] = row
			order = append(order, k)
		}
		row.Quantity += event.Quantity
		switch event.Status {
		case store.EventBilled:
			row.Billed += event.Quantity
		case store.EventRecorded:
			row.Recorded += event.Quantity
		case store.EventArchived:
			row.Archived += event.Quantity
		}
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].subscription != order[j].subscription {
			return order[i].subscription < order[j].subscription
		}
		return order[i].metric < order[j].metric
	})
	out := make([]Report, 0, len(order))
	for _, k := range order {
		out = append(out, *rows[k])
	}
	return out
}

// BillingState is whether a subscription is inside, above or below what its plan includes.
type BillingState string

// The states a metered quantity can be in against a plan.
const (
	// WithinPlan is a quantity the plan's price already covers.
	WithinPlan BillingState = "within_plan"
	// AbovePlan is a quantity the customer pays per unit for.
	AbovePlan BillingState = "above_plan"
	// Unlimited is a plan that does not meter.
	Unlimited BillingState = "unlimited"
)

// State reports where a quantity stands against a plan.
func State(plan store.Plan, used int64) BillingState {
	if plan.IncludedQuantity < 0 {
		return Unlimited
	}
	if used <= plan.IncludedQuantity {
		return WithinPlan
	}
	return AbovePlan
}
