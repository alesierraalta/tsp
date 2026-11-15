// Package reconcile checks the books against themselves: the lines of an invoice against its total,
// the payments against what each invoice owes, and the invoices against the tenant's reported revenue.
// It writes nothing; it is what an operator runs when a number in a report does not match the number
// in a ledger.
package reconcile

import (
	"fmt"
	"sort"

	"example.com/billing/internal/clock"
	"example.com/billing/internal/invoicing"
	"example.com/billing/internal/money"
	"example.com/billing/internal/store"
)

// Finding is one disagreement the ledger found.
type Finding struct {
	Invoice string
	Kind    string
	Detail  string
}

// Report is the result of one reconciliation.
type Report struct {
	Tenant      string
	Invoices    int
	Lines       int
	Charged     money.Amount
	Collected   money.Amount
	Outstanding money.Amount
	Findings    []Finding
}

// Clean reports whether the books agreed.
func (r Report) Clean() bool { return len(r.Findings) == 0 }

// Ledger reconciles the invoices of one tenant against their lines and their payments.
func Ledger(tenant string, invoices []store.Invoice, payments []store.Payment) (Report, error) {
	report := Report{Tenant: tenant}
	currency := ""
	paid := make(map[string]money.Amount)
	for _, payment := range payments {
		if payment.Tenant != tenant {
			continue
		}
		current, ok := paid[payment.Invoice]
		if !ok {
			current = payment.Amount
		} else {
			next, err := current.Add(payment.Amount)
			if err != nil {
				return Report{}, err
			}
			current = next
		}
		paid[payment.Invoice] = current
	}
	for _, invoice := range invoices {
		if invoice.Tenant != tenant {
			continue
		}
		if currency == "" {
			currency = invoice.Currency
		}
		if invoice.Currency != currency {
			report.Findings = append(report.Findings, Finding{
				Invoice: invoice.ID, Kind: "currency",
				Detail: fmt.Sprintf("invoice is in %s and the tenant bills in %s", invoice.Currency, currency),
			})
			continue
		}
		if err := invoicing.Reconcile(invoice); err != nil {
			report.Findings = append(report.Findings, Finding{
				Invoice: invoice.ID, Kind: "lines", Detail: err.Error(),
			})
		}
		report.Invoices++
		report.Lines += len(invoice.Lines)
		report.Charged, _ = addTo(report.Charged, invoice.Total)
		collected, ok := paid[invoice.ID]
		if !ok {
			collected = money.Zero(currency)
		}
		report.Collected, _ = addTo(report.Collected, collected)
		outstanding, err := invoicing.Outstanding(invoice)
		if err != nil {
			return Report{}, err
		}
		report.Outstanding, _ = addTo(report.Outstanding, outstanding)
		if cmp, err := collected.Cmp(invoice.Paid); err == nil && cmp != 0 {
			report.Findings = append(report.Findings, Finding{
				Invoice: invoice.ID, Kind: "payments",
				Detail: fmt.Sprintf("invoice records %s paid and the ledger carries %s", invoice.Paid, collected),
			})
		}
	}
	return report, nil
}

// addTo adds an amount to a running total, initialising the total with the amount's currency.
func addTo(total, amount money.Amount) (money.Amount, error) {
	if total.Currency == "" {
		return amount, nil
	}
	return total.Add(amount)
}

// RevenueLine is one day's revenue the reconciliation compares the invoices against.
type RevenueLine struct {
	Day      int64
	Total    money.Amount
	Invoices int
}

// Revenue returns the invoiced total per UTC day, ordered by day, for an operator comparing the
// service's own numbers with the figures a finance system reports.
func Revenue(invoices []store.Invoice) ([]RevenueLine, error) {
	byDay := make(map[int64]*RevenueLine)
	var days []int64
	for _, invoice := range invoices {
		day := clock.DayNumber(invoice.Issued)
		line, ok := byDay[day]
		if !ok {
			line = &RevenueLine{Day: day, Total: money.Zero(invoice.Currency)}
			byDay[day] = line
			days = append(days, day)
		}
		next, err := line.Total.Add(invoice.Total)
		if err != nil {
			return nil, err
		}
		line.Total = next
		line.Invoices++
	}
	sort.Slice(days, func(i, j int) bool { return days[i] < days[j] })
	out := make([]RevenueLine, 0, len(days))
	for _, day := range days {
		out = append(out, *byDay[day])
	}
	return out, nil
}

// Describe renders a finding the way the console prints it.
func Describe(f Finding) string {
	return fmt.Sprintf("%s: %s: %s", f.Invoice, f.Kind, f.Detail)
}
