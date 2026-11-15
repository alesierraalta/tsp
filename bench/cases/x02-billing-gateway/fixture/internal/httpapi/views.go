package httpapi

import (
	"time"

	"example.com/billing/internal/clock"
	"example.com/billing/internal/money"
	"example.com/billing/internal/proration"
	"example.com/billing/internal/store"
)

// factorOf renders the part of a cycle that is left at an instant, for a response and a description.
func factorOf(period clock.Period, at time.Time) string {
	factor, err := proration.Remaining(period, at)
	if err != nil {
		return "0/0"
	}
	return factor.String()
}

// amountView is money as a response carries it: the minor units an operator reconciles with and the
// text a customer reads. Both travel, so a client never has to decide which one it received.
type amountView struct {
	Minor    int64  `json:"minor"`
	Currency string `json:"currency"`
	Display  string `json:"display"`
}

// newAmountView renders an amount.
func newAmountView(amount money.Amount) amountView {
	return amountView{Minor: amount.Minor, Currency: amount.Currency, Display: amount.String()}
}

// lineView is one invoice line as a response carries it.
type lineView struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Description string     `json:"description"`
	Quantity    int64      `json:"quantity"`
	UnitPrice   amountView `json:"unit_price"`
	Amount      amountView `json:"amount"`
}

// newLineView renders a line.
func newLineView(line store.Line) lineView {
	return lineView{
		ID: line.ID, Kind: line.Kind, Description: line.Description, Quantity: line.Quantity,
		UnitPrice: newAmountView(line.UnitPrice), Amount: newAmountView(line.Amount),
	}
}

// invoiceView is one invoice as a response carries it.
type invoiceView struct {
	ID           string     `json:"id"`
	Account      string     `json:"account"`
	Subscription string     `json:"subscription"`
	Status       string     `json:"status"`
	Period       string     `json:"period"`
	Issued       string     `json:"issued"`
	Due          string     `json:"due"`
	Currency     string     `json:"currency"`
	Total        amountView `json:"total"`
	Paid         amountView `json:"paid"`
	Remaining    amountView `json:"remaining"`
	Lines        []lineView `json:"lines"`
}

// newInvoiceView renders an invoice and the lines it carries.
func newInvoiceView(invoice store.Invoice) invoiceView {
	remaining, err := invoice.Total.Sub(invoice.Paid)
	if err != nil {
		remaining = money.Amount{Currency: invoice.Currency}
	}
	lines := make([]lineView, 0, len(invoice.Lines))
	for _, line := range invoice.Lines {
		lines = append(lines, newLineView(line))
	}
	return invoiceView{
		ID: invoice.ID, Account: invoice.Account, Subscription: invoice.Subscription,
		Status: string(invoice.Status), Period: invoice.Period.String(),
		Issued: invoice.Issued.Format("2006-01-02"), Due: invoice.Due.Format("2006-01-02"),
		Currency: invoice.Currency, Total: newAmountView(invoice.Total),
		Paid: newAmountView(invoice.Paid), Remaining: newAmountView(remaining), Lines: lines,
	}
}

// creditView is one credit as a response carries it.
type creditView struct {
	ID     string     `json:"id"`
	Amount amountView `json:"amount"`
	Reason string     `json:"reason"`
}

// newCreditView renders a credit.
func newCreditView(credit store.Credit) creditView {
	return creditView{ID: credit.ID, Amount: newAmountView(credit.Amount), Reason: credit.Reason}
}
