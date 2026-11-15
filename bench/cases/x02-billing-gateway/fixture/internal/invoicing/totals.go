package invoicing

import (
	"fmt"

	"example.com/billing/internal/money"
	"example.com/billing/internal/store"
)

// Total adds every line of an invoice. The lines of one invoice are all in the invoice's currency; a
// line in another currency is a refusal, because the sum of two currencies is not a number.
func Total(lines []store.Line) (money.Amount, error) {
	if len(lines) == 0 {
		return money.Amount{}, fmt.Errorf("invoicing: an invoice needs at least one line")
	}
	currency := lines[0].Amount.Currency
	total := money.Amount{Minor: 0, Currency: currency}
	for _, line := range lines {
		if line.Amount.Currency != currency {
			return money.Amount{}, fmt.Errorf("%w: line %q is in %s and the invoice in %s",
				money.ErrMismatch, line.ID, line.Amount.Currency, currency)
		}
		next, err := total.Add(line.Amount)
		if err != nil {
			return money.Amount{}, err
		}
		total = next
	}
	return total, nil
}

// Taxable returns the total of the lines above the tax line: the invoice's total without its tax and
// without its credit, which is the base a tax rate is applied to.
func Taxable(lines []store.Line) (money.Amount, error) {
	selected := make([]store.Line, 0, len(lines))
	for _, line := range lines {
		if line.Kind == KindTax || line.Kind == KindCredit {
			continue
		}
		selected = append(selected, line)
	}
	if len(selected) == 0 {
		return money.Amount{}, fmt.Errorf("invoicing: there is nothing to tax")
	}
	return Total(selected)
}

// Reconcile checks that an invoice's total is the sum of its lines and that its paid amount does not
// exceed it. The store validates the same invariant when the invoice is written; this is the check a
// report runs over invoices that were written by an older version.
func Reconcile(invoice store.Invoice) error {
	if len(invoice.Lines) == 0 {
		return fmt.Errorf("invoicing: invoice %q carries no lines", invoice.ID)
	}
	sum, err := Total(invoice.Lines)
	if err != nil {
		return err
	}
	if cmp, err := sum.Cmp(invoice.Total); err != nil {
		return err
	} else if cmp != 0 {
		return fmt.Errorf("invoicing: invoice %q totals %s but its lines add up to %s",
			invoice.ID, invoice.Total, sum)
	}
	if cmp, err := invoice.Paid.Cmp(invoice.Total); err != nil {
		return err
	} else if cmp > 0 {
		return fmt.Errorf("invoicing: invoice %q is paid %s of %s",
			invoice.ID, invoice.Paid, invoice.Total)
	}
	return nil
}

// Outstanding returns what an invoice still owes.
func Outstanding(invoice store.Invoice) (money.Amount, error) {
	return invoice.Total.Sub(invoice.Paid)
}

// Split returns the shares of an invoice's total over the accounts given, which is how a consolidated
// bill for several accounts of one tenant is divided. The shares add up to the total exactly.
func Split(invoice store.Invoice, accounts int) ([]money.Amount, error) {
	if accounts <= 0 {
		return nil, fmt.Errorf("invoicing: split needs a positive number of accounts")
	}
	return money.SplitEvenly(invoice.Total, accounts)
}

// Summary is the one line an operation reads about an invoice.
type Summary struct {
	Invoice   string
	Status    store.InvoiceStatus
	Total     money.Amount
	Paid      money.Amount
	Remaining money.Amount
	Lines     int
}

// Summarize returns the summary of an invoice.
func Summarize(invoice store.Invoice) (Summary, error) {
	remaining, err := Outstanding(invoice)
	if err != nil {
		return Summary{}, err
	}
	return Summary{
		Invoice:   invoice.ID,
		Status:    invoice.Status,
		Total:     invoice.Total,
		Paid:      invoice.Paid,
		Remaining: remaining,
		Lines:     len(invoice.Lines),
	}, nil
}

// MarkPaid returns the invoice with a payment applied and its status moved to paid. The store performs
// the same transition when a payment is recorded; this is the shape a caller that already has the
// payment uses.
func MarkPaid(invoice store.Invoice, payment money.Amount) (store.Invoice, error) {
	paid, err := invoice.Paid.Add(payment)
	if err != nil {
		return store.Invoice{}, err
	}
	if cmp, err := paid.Cmp(invoice.Total); err != nil {
		return store.Invoice{}, err
	} else if cmp > 0 {
		return store.Invoice{}, fmt.Errorf("invoicing: a payment of %s exceeds the %s owed on %q",
			payment, invoice.Total, invoice.ID)
	}
	invoice.Paid = paid
	if cmp, err := paid.Cmp(invoice.Total); err == nil && cmp == 0 {
		invoice.Status = store.InvoicePaid
	}
	return invoice, nil
}
