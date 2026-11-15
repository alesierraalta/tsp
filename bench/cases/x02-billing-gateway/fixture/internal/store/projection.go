package store

import (
	"time"

	"example.com/billing/internal/clock"
	"example.com/billing/internal/sqlrun"
)

// The projection's tables. A report names one of these and the columns it carries, and the report
// definitions in internal/reports read nothing else.
const (
	// tableUsage is the usage events, one row per reported quantity.
	tableUsage = "usage_events"
	// tableInvoices is the invoices, one row per bill.
	tableInvoices = "invoices"
	// tableSubscriptions is the subscriptions, one row per account's use of a plan.
	tableSubscriptions = "subscriptions"
	// tablePlans is the price list, one row per plan.
	tablePlans = "plans"
	// tableAccounts is the account directory of every tenant.
	tableAccounts = "accounts"
)

// createTables declares the projection's tables and their column kinds.
func (s *Store) createTables() error {
	definitions := []struct {
		name    string
		columns []sqlrun.Column
	}{
		{tableUsage, []sqlrun.Column{
			{Name: "id", Kind: sqlrun.Text},
			{Name: "tenant", Kind: sqlrun.Text},
			{Name: "account", Kind: sqlrun.Text},
			{Name: "subscription", Kind: sqlrun.Text},
			{Name: "metric", Kind: sqlrun.Text},
			{Name: "quantity", Kind: sqlrun.Number},
			{Name: "status", Kind: sqlrun.Text},
			{Name: "day", Kind: sqlrun.Number},
		}},
		{tableInvoices, []sqlrun.Column{
			{Name: "id", Kind: sqlrun.Text},
			{Name: "tenant", Kind: sqlrun.Text},
			{Name: "account", Kind: sqlrun.Text},
			{Name: "subscription", Kind: sqlrun.Text},
			{Name: "status", Kind: sqlrun.Text},
			{Name: "currency", Kind: sqlrun.Text},
			{Name: "total", Kind: sqlrun.Number},
			{Name: "paid", Kind: sqlrun.Number},
			{Name: "period_start", Kind: sqlrun.Number},
			{Name: "period_end", Kind: sqlrun.Number},
			{Name: "issued_day", Kind: sqlrun.Number},
			{Name: "due_day", Kind: sqlrun.Number},
		}},
		{tableSubscriptions, []sqlrun.Column{
			{Name: "id", Kind: sqlrun.Text},
			{Name: "tenant", Kind: sqlrun.Text},
			{Name: "account", Kind: sqlrun.Text},
			{Name: "plan", Kind: sqlrun.Text},
			{Name: "plan_code", Kind: sqlrun.Text},
			{Name: "seats", Kind: sqlrun.Number},
			{Name: "status", Kind: sqlrun.Text},
			{Name: "seat_price", Kind: sqlrun.Number},
			{Name: "base_price", Kind: sqlrun.Number},
			{Name: "anchor_day", Kind: sqlrun.Number},
		}},
		{tablePlans, []sqlrun.Column{
			{Name: "id", Kind: sqlrun.Text},
			{Name: "tenant", Kind: sqlrun.Text},
			{Name: "code", Kind: sqlrun.Text},
			{Name: "name", Kind: sqlrun.Text},
			{Name: "base_price", Kind: sqlrun.Number},
			{Name: "seat_price", Kind: sqlrun.Number},
			{Name: "unit_price", Kind: sqlrun.Number},
			{Name: "included_seats", Kind: sqlrun.Number},
			{Name: "metric", Kind: sqlrun.Text},
		}},
		{tableAccounts, []sqlrun.Column{
			{Name: "id", Kind: sqlrun.Text},
			{Name: "tenant", Kind: sqlrun.Text},
			{Name: "email", Kind: sqlrun.Text},
			{Name: "name", Kind: sqlrun.Text},
		}},
	}
	for _, definition := range definitions {
		if err := s.db.Create(definition.name, definition.columns...); err != nil {
			return err
		}
	}
	return nil
}

// refresh rebuilds every projection table from the records. It runs under the store's write lock, so
// a reader sees the old projection or the new one and never a table that is half of each.
func (s *Store) refresh() error {
	if err := s.refreshPlans(); err != nil {
		return err
	}
	if err := s.refreshAccounts(); err != nil {
		return err
	}
	if err := s.refreshSubscriptions(); err != nil {
		return err
	}
	if err := s.refreshInvoices(); err != nil {
		return err
	}
	return s.refreshUsage()
}

// refreshPlans rewrites the plans table.
func (s *Store) refreshPlans() error {
	plans := sortedByID(s.plans, func(p Plan) string { return p.ID })
	rows := make([][]sqlrun.Value, 0, len(plans))
	for _, plan := range plans {
		rows = append(rows, []sqlrun.Value{
			textValue(plan.ID), textValue(plan.Tenant), textValue(plan.Code), textValue(plan.Name),
			numberValue(plan.BasePrice.Minor), numberValue(plan.SeatPrice.Minor),
			numberValue(plan.UnitPrice.Minor), numberValue(int64(plan.IncludedSeats)),
			textValue(plan.Metric),
		})
	}
	return s.db.Replace(tablePlans, rows)
}

// refreshAccounts rewrites the accounts table.
func (s *Store) refreshAccounts() error {
	accounts := sortedByID(s.accounts, func(a Account) string { return a.ID })
	rows := make([][]sqlrun.Value, 0, len(accounts))
	for _, account := range accounts {
		rows = append(rows, []sqlrun.Value{
			textValue(account.ID), textValue(account.Tenant),
			textValue(account.Email), textValue(account.Name),
		})
	}
	return s.db.Replace(tableAccounts, rows)
}

// refreshSubscriptions rewrites the subscriptions table, joining each subscription's plan so a report
// reads a price without a second statement.
func (s *Store) refreshSubscriptions() error {
	subs := sortedByID(s.subscriptions, func(sub Subscription) string { return sub.ID })
	rows := make([][]sqlrun.Value, 0, len(subs))
	for _, sub := range subs {
		plan := s.plans[sub.Plan]
		rows = append(rows, []sqlrun.Value{
			textValue(sub.ID), textValue(sub.Tenant), textValue(sub.Account),
			textValue(sub.Plan), textValue(plan.Code), numberValue(int64(sub.Seats)),
			textValue(string(sub.Status)), numberValue(plan.SeatPrice.Minor),
			numberValue(plan.BasePrice.Minor), numberValue(clock.DayNumber(sub.Anchor)),
		})
	}
	return s.db.Replace(tableSubscriptions, rows)
}

// refreshInvoices rewrites the invoices table.
func (s *Store) refreshInvoices() error {
	invoices := sortedByID(s.invoices, func(i Invoice) string { return i.ID })
	rows := make([][]sqlrun.Value, 0, len(invoices))
	for _, invoice := range invoices {
		rows = append(rows, []sqlrun.Value{
			textValue(invoice.ID), textValue(invoice.Tenant), textValue(invoice.Account),
			textValue(invoice.Subscription), textValue(string(invoice.Status)),
			textValue(invoice.Currency), numberValue(invoice.Total.Minor),
			numberValue(invoice.Paid.Minor), numberValue(clock.DayNumber(invoice.Period.Start)),
			numberValue(clock.DayNumber(invoice.Period.End)), numberValue(clock.DayNumber(invoice.Issued)),
			numberValue(clock.DayNumber(invoice.Due)),
		})
	}
	return s.db.Replace(tableInvoices, rows)
}

// refreshUsage rewrites the usage table.
func (s *Store) refreshUsage() error {
	events := sortedByID(s.events, func(e UsageEvent) string { return e.ID })
	rows := make([][]sqlrun.Value, 0, len(events))
	for _, event := range events {
		rows = append(rows, []sqlrun.Value{
			textValue(event.ID), textValue(event.Tenant), textValue(event.Account),
			textValue(event.Subscription), textValue(event.Metric),
			numberValue(event.Quantity), textValue(string(event.Status)),
			numberValue(clock.DayNumber(event.At)),
		})
	}
	return s.db.Replace(tableUsage, rows)
}

// stat is one aggregate the service reports about itself.
type stat struct {
	Name  string
	Count int
}

// Stats lists the number of records the store carries, in a stable order, for the operator console.
func (s *Store) Stats() []stat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return []stat{
		{Name: "tenants", Count: len(s.tenants)},
		{Name: "plans", Count: len(s.plans)},
		{Name: "accounts", Count: len(s.accounts)},
		{Name: "subscriptions", Count: len(s.subscriptions)},
		{Name: "invoices", Count: len(s.invoices)},
		{Name: "usage_events", Count: len(s.events)},
		{Name: "credits", Count: len(s.credits)},
		{Name: "payments", Count: len(s.payments)},
	}
}

// issuedDay returns the UTC day an instant falls on.
func issuedDay(t time.Time) int64 { return clock.DayNumber(t) }
