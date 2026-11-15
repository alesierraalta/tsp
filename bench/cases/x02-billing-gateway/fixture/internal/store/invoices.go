package store

import (
	"fmt"
	"sort"

	"example.com/billing/internal/money"
)

// UpdateInvoice replaces an invoice the store carries.
func (s *Store) UpdateInvoice(invoice Invoice) error {
	if err := invoice.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.invoices[invoice.ID]; !exists {
		return fmt.Errorf("%w: invoice %q", ErrNotFound, invoice.ID)
	}
	s.invoices[invoice.ID] = invoice
	return s.refreshInvoices()
}

// Invoice returns one invoice.
func (s *Store) Invoice(id string) (Invoice, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	invoice, ok := s.invoices[id]
	if !ok {
		return Invoice{}, fmt.Errorf("%w: invoice %q", ErrNotFound, id)
	}
	return invoice, nil
}

// InvoiceInTenant returns an invoice only when it belongs to the tenant given.
func (s *Store) InvoiceInTenant(tenant, id string) (Invoice, error) {
	invoice, err := s.Invoice(id)
	if err != nil {
		return Invoice{}, err
	}
	if invoice.Tenant != tenant {
		return Invoice{}, fmt.Errorf("%w: invoice %q in tenant %q", ErrNotFound, id, tenant)
	}
	return invoice, nil
}

// Invoices returns the invoices of a tenant in id order.
func (s *Store) Invoices(tenant string) []Invoice {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Invoice, 0, len(s.invoices))
	for _, invoice := range s.invoices {
		if invoice.Tenant == tenant {
			out = append(out, invoice)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// InvoicesOf returns the invoices of one account.
func (s *Store) InvoicesOf(tenant, account string) []Invoice {
	all := s.Invoices(tenant)
	out := all[:0]
	for _, invoice := range all {
		if invoice.Account == account {
			out = append(out, invoice)
		}
	}
	return out
}

// InvoiceFor returns the invoice a subscription was billed through for a period, if there is one. A
// subscription is billed once per cycle, so a second call for the same cycle is a duplicate rather
// than a second bill.
func (s *Store) InvoiceFor(tenant, subscription string, periodDay int64) (Invoice, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, invoice := range s.invoices {
		if invoice.Tenant != tenant || invoice.Subscription != subscription {
			continue
		}
		if issuedDay(invoice.Period.Start) == periodDay {
			return invoice, true
		}
	}
	return Invoice{}, false
}

// AddPayment applies a payment to an invoice. The invoice is paid when the payments cover its total,
// and a payment above the total is refused: money the invoice does not ask for is a credit to issue
// separately, not a correction to hide inside a paid bill.
func (s *Store) AddPayment(payment Payment) error {
	if err := payment.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	invoice, ok := s.invoices[payment.Invoice]
	if !ok || invoice.Tenant != payment.Tenant {
		return fmt.Errorf("%w: invoice %q in tenant %q", ErrNotFound, payment.Invoice, payment.Tenant)
	}
	if invoice.Status == InvoiceVoid {
		return fmt.Errorf("%w: invoice %q is void", ErrInvalid, invoice.ID)
	}
	if payment.Amount.Currency != invoice.Currency {
		return fmt.Errorf("%w: payment is in %s and invoice %q in %s",
			money.ErrMismatch, payment.Amount.Currency, invoice.ID, invoice.Currency)
	}
	paid, err := invoice.Paid.Add(payment.Amount)
	if err != nil {
		return err
	}
	if cmp, err := paid.Cmp(invoice.Total); err != nil {
		return err
	} else if cmp > 0 {
		return fmt.Errorf("%w: payment %s exceeds the %s invoice %q owes",
			ErrInvalid, payment.Amount, invoice.Total, invoice.ID)
	}
	if _, exists := s.payments[payment.ID]; exists {
		return fmt.Errorf("%w: payment %q", ErrDuplicate, payment.ID)
	}
	s.payments[payment.ID] = payment
	invoice.Paid = paid
	if cmp, err := paid.Cmp(invoice.Total); err == nil && cmp == 0 {
		invoice.Status = InvoicePaid
	}
	s.invoices[invoice.ID] = invoice
	if err := s.refreshInvoices(); err != nil {
		return err
	}
	return s.refresh()
}

// Payment returns one payment.
func (s *Store) Payment(id string) (Payment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	payment, ok := s.payments[id]
	if !ok {
		return Payment{}, fmt.Errorf("%w: payment %q", ErrNotFound, id)
	}
	return payment, nil
}

// PaymentsOf returns the payments applied to an invoice.
func (s *Store) PaymentsOf(tenant, invoice string) []Payment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Payment, 0, len(s.payments))
	for _, payment := range s.payments {
		if payment.Tenant == tenant && payment.Invoice == invoice {
			out = append(out, payment)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// AddCredit records money an account is owed.
func (s *Store) AddCredit(credit Credit) error {
	if err := credit.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureTenant(credit.Tenant); err != nil {
		return err
	}
	if _, exists := s.credits[credit.ID]; exists {
		return fmt.Errorf("%w: credit %q", ErrDuplicate, credit.ID)
	}
	s.credits[credit.ID] = credit
	return nil
}

// CreditsOf returns the credits of an account in id order.
func (s *Store) CreditsOf(tenant, account string) []Credit {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Credit, 0, len(s.credits))
	for _, credit := range s.credits {
		if credit.Tenant == tenant && credit.Account == account {
			out = append(out, credit)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// CreditBalance returns the sum of the credits of an account.
func (s *Store) CreditBalance(tenant, account string) (money.Amount, error) {
	credits := s.CreditsOf(tenant, account)
	if len(credits) == 0 {
		return money.Zero("EUR"), nil
	}
	total := money.Zero(credits[0].Amount.Currency)
	for _, credit := range credits {
		next, err := total.Add(credit.Amount)
		if err != nil {
			return money.Amount{}, err
		}
		total = next
	}
	return total, nil
}

// OpenInvoices returns the invoices of a tenant that are still owed, in id order.
func (s *Store) OpenInvoices(tenant string) []Invoice {
	all := s.Invoices(tenant)
	out := make([]Invoice, 0, len(all))
	for _, invoice := range all {
		if invoice.Status == InvoiceOpen || invoice.Status == InvoiceDraft {
			out = append(out, invoice)
		}
	}
	return out
}
