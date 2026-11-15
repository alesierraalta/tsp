package store

import (
	"testing"
	"time"

	"example.com/billing/internal/clock"
	"example.com/billing/internal/config"
	"example.com/billing/internal/money"
)

// The identifiers these tests read. They are fixed rather than minted so a test can name the record
// it asserts about.
const (
	tenantAcme   = "ten-00000000000000000000000a"
	tenantGlobex = "ten-00000000000000000000000b"
	accountAcme  = "acc-000000000000000000000001"
	accountOther = "acc-000000000000000000000002"
	accountGlob  = "acc-000000000000000000000003"
	planStarter  = "pln-000000000000000000000001"
	planScale    = "pln-000000000000000000000002"
	planGlob     = "pln-000000000000000000000003"
	subAcme      = "sub-000000000000000000000001"
	subOther     = "sub-000000000000000000000002"
	subGlob      = "sub-000000000000000000000003"
	invoiceOpen  = "inv-000000000000000000000001"
	invoicePaid  = "inv-000000000000000000000002"
)

// open returns a store with a pinned clock and a small dataset built through its own API.
func open(t *testing.T) *Store {
	t.Helper()
	now := clock.Fixed(time.Date(2024, 4, 24, 9, 0, 0, 0, time.UTC))
	s, err := Open(config.Default().Limits(), now)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := seed(t, s, now()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return s
}

// seed writes two tenants, three plans, three accounts and three subscriptions, one usage event and
// two invoices.
func seed(t *testing.T, s *Store, at time.Time) error {
	t.Helper()
	for _, tenant := range []Tenant{
		{ID: tenantAcme, Name: "Acme Logistics", Created: at.AddDate(0, -3, 0)},
		{ID: tenantGlobex, Name: "Globex Shipping", Created: at.AddDate(0, -2, 0)},
	} {
		if err := s.PutTenant(tenant); err != nil {
			return err
		}
	}
	for _, plan := range []Plan{
		{
			ID: planStarter, Tenant: tenantAcme, Code: "starter", Name: "Starter",
			BasePrice: testAmount(t, 4900), SeatPrice: testAmount(t, 1000),
			UnitPrice: testAmount(t, 2), IncludedSeats: 2, IncludedQuantity: 1000,
			Metric: "requests", Active: true,
		},
		{
			ID: planScale, Tenant: tenantAcme, Code: "scale", Name: "Scale",
			BasePrice: testAmount(t, 19900), SeatPrice: testAmount(t, 1500),
			UnitPrice: testAmount(t, 1), IncludedSeats: 10, IncludedQuantity: 10000,
			Metric: "requests", Active: true,
		},
		{
			ID: planGlob, Tenant: tenantGlobex, Code: "starter", Name: "Starter",
			BasePrice: testAmount(t, 3900), SeatPrice: testAmount(t, 900),
			UnitPrice: testAmount(t, 2), IncludedSeats: 1, IncludedQuantity: 500,
			Metric: "requests", Active: true,
		},
	} {
		if err := s.PutPlan(plan); err != nil {
			return err
		}
	}
	for _, account := range []Account{
		{ID: accountAcme, Tenant: tenantAcme, Email: "ops@acme.example", Name: "Acme Operations", Created: at},
		{ID: accountOther, Tenant: tenantAcme, Email: "billing@acme.example", Name: "Acme Billing", Created: at},
		{ID: accountGlob, Tenant: tenantGlobex, Email: "ops@globex.example", Name: "Globex Operations", Created: at},
	} {
		if err := s.PutAccount(account); err != nil {
			return err
		}
	}
	anchor := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, time.UTC)
	for _, sub := range []Subscription{
		{ID: subAcme, Tenant: tenantAcme, Account: accountAcme, Plan: planStarter, Seats: 5,
			Anchor: anchor, Status: StatusActive, UpdatedAt: at},
		{ID: subOther, Tenant: tenantAcme, Account: accountOther, Plan: planScale, Seats: 12,
			Anchor: anchor, Status: StatusActive, UpdatedAt: at},
		{ID: subGlob, Tenant: tenantGlobex, Account: accountGlob, Plan: planGlob, Seats: 4,
			Anchor: anchor, Status: StatusActive, UpdatedAt: at},
	} {
		if err := s.PutSubscription(sub); err != nil {
			return err
		}
	}
	for i, event := range []UsageEvent{
		{ID: "evt-000000000000000000000001", Tenant: tenantAcme, Account: accountAcme,
			Subscription: subAcme, Metric: "requests", Quantity: 1200,
			At: anchor.AddDate(0, 0, 1), Status: EventRecorded},
		{ID: "evt-000000000000000000000002", Tenant: tenantAcme, Account: accountAcme,
			Subscription: subAcme, Metric: "requests", Quantity: 800,
			At: anchor.AddDate(0, 0, 2), Status: EventBilled},
		{ID: "evt-000000000000000000000003", Tenant: tenantAcme, Account: accountOther,
			Subscription: subOther, Metric: "requests", Quantity: 5400,
			At: anchor.AddDate(0, 0, 2), Status: EventArchived},
		{ID: "evt-000000000000000000000004", Tenant: tenantGlobex, Account: accountGlob,
			Subscription: subGlob, Metric: "requests", Quantity: 300,
			At: anchor.AddDate(0, 0, 1), Status: EventRecorded},
	} {
		_ = i
		if err := s.AddEvent(event); err != nil {
			return err
		}
	}
	period, err := clock.Cycle(anchor, 1)
	if err != nil {
		return err
	}
	for _, invoice := range []Invoice{
		{
			ID: invoiceOpen, Tenant: tenantAcme, Account: accountAcme, Subscription: subAcme,
			Period: period, Issued: period.End, Due: period.End.AddDate(0, 0, 14),
			Status: InvoiceOpen, Currency: "EUR",
			Lines: []Line{{
				ID: "lin-000000000000000000000001", Kind: "base", Description: "Starter base price",
				Quantity: 1, UnitPrice: testAmount(t, 7900), Amount: testAmount(t, 7900),
			}},
			Total: testAmount(t, 7900), Paid: testAmount(t, 0),
		},
		{
			ID: invoicePaid, Tenant: tenantAcme, Account: accountOther, Subscription: subOther,
			Period: period, Issued: period.End, Due: period.End.AddDate(0, 0, 14),
			Status: InvoicePaid, Currency: "EUR",
			Lines: []Line{{
				ID: "lin-000000000000000000000002", Kind: "base", Description: "Scale base price",
				Quantity: 1, UnitPrice: testAmount(t, 22900), Amount: testAmount(t, 22900),
			}},
			Total: testAmount(t, 22900), Paid: testAmount(t, 22900),
		},
	} {
		if err := s.PutInvoice(invoice); err != nil {
			return err
		}
	}
	return nil
}

func TestProjectionCarriesTheRecords(t *testing.T) {
	s := open(t)
	if got := len(s.Tenants()); got != 2 {
		t.Fatalf("tenants = %d, want 2", got)
	}
	if got := s.Count(tableUsage); got != 4 {
		t.Fatalf("usage rows = %d, want 4", got)
	}
	if got := s.Count(tableInvoices); got != 2 {
		t.Fatalf("invoice rows = %d, want 2", got)
	}
	if got := s.Count(tableSubscriptions); got != 3 {
		t.Fatalf("subscription rows = %d, want 3", got)
	}
	if got := s.Count(tablePlans); got != 3 {
		t.Fatalf("plan rows = %d, want 3", got)
	}
	if got := s.Count(tableAccounts); got != 3 {
		t.Fatalf("account rows = %d, want 3", got)
	}
}

func TestRecordsAreScopedToATenant(t *testing.T) {
	s := open(t)
	if got := len(s.Subscriptions(tenantAcme)); got != 2 {
		t.Fatalf("acme subscriptions = %d, want 2", got)
	}
	if got := len(s.Subscriptions(tenantGlobex)); got != 1 {
		t.Fatalf("globex subscriptions = %d, want 1", got)
	}
	if _, err := s.SubscriptionInTenant(tenantAcme, subGlob); err == nil {
		t.Fatal("a subscription of another tenant was resolved")
	}
	if _, err := s.AccountInTenant(tenantGlobex, accountAcme); err == nil {
		t.Fatal("an account of another tenant was resolved")
	}
	if _, err := s.InvoiceInTenant(tenantGlobex, invoiceOpen); err == nil {
		t.Fatal("an invoice of another tenant was resolved")
	}
}

func TestInvoiceValidationRefusesALineThatDoesNotAddUp(t *testing.T) {
	s := open(t)
	invoice, err := s.Invoice(invoiceOpen)
	if err != nil {
		t.Fatalf("invoice: %v", err)
	}
	invoice.Total = testAmount(t, 1)
	if err := invoice.Validate(); err == nil {
		t.Fatal("an invoice whose lines do not add to its total was accepted")
	}
}

func TestPutRefusesADuplicate(t *testing.T) {
	s := open(t)
	plan, err := s.Plan(planStarter)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if err := s.PutPlan(plan); err == nil {
		t.Fatal("a duplicate plan was accepted")
	}
	account, err := s.Account(accountAcme)
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	if err := s.PutAccount(account); err == nil {
		t.Fatal("a duplicate account was accepted")
	}
}

func TestUsageEventsAggregateIntoAProjectionRow(t *testing.T) {
	s := open(t)
	now := time.Date(2024, 4, 24, 9, 0, 0, 0, time.UTC)
	added := UsageEvent{
		ID: "evt-00000000000000000000000f", Tenant: tenantAcme, Account: accountAcme,
		Subscription: subAcme, Metric: "requests", Quantity: 42, At: now, Status: EventRecorded,
	}
	if err := s.AddEvent(added); err != nil {
		t.Fatalf("add: %v", err)
	}
	if got := s.Count(tableUsage); got != 5 {
		t.Fatalf("usage rows = %d, want 5", got)
	}
	total := s.UsageTotal(tenantAcme, subAcme, "requests", mustPeriod(t, "2024-04-01", "2024-05-01"))
	if total != 2042 {
		t.Fatalf("usage total = %d, want 2042", total)
	}
	billed, err := s.MarkBilled(tenantAcme, subAcme, mustPeriod(t, "2024-04-01", "2024-05-01"))
	if err != nil || billed != 2 {
		t.Fatalf("billed = %d err=%v, want 2", billed, err)
	}
}

func TestPaymentSettlesAnInvoice(t *testing.T) {
	s := open(t)
	invoice, err := s.Invoice(invoiceOpen)
	if err != nil {
		t.Fatalf("invoice: %v", err)
	}
	payment := Payment{
		ID: "pay-000000000000000000000001", Tenant: invoice.Tenant, Invoice: invoice.ID,
		Amount: invoice.Total, At: time.Date(2024, 4, 24, 9, 0, 0, 0, time.UTC),
	}
	if err := s.AddPayment(payment); err != nil {
		t.Fatalf("payment: %v", err)
	}
	updated, err := s.Invoice(invoice.ID)
	if err != nil {
		t.Fatalf("invoice: %v", err)
	}
	if updated.Status != InvoicePaid {
		t.Fatalf("status = %q, want paid", updated.Status)
	}
	over := Payment{
		ID: "pay-000000000000000000000002", Tenant: invoice.Tenant, Invoice: invoice.ID,
		Amount: testAmount(t, 100), At: time.Date(2024, 4, 24, 9, 0, 0, 0, time.UTC),
	}
	if err := s.AddPayment(over); err == nil {
		t.Fatal("a payment on a settled invoice was accepted")
	}
}

// mustAmount returns an amount in EUR.
func testAmount(t *testing.T, minor int64) money.Amount {
	t.Helper()
	amount, err := money.New(minor, "EUR")
	if err != nil {
		t.Fatalf("amount: %v", err)
	}
	return amount
}

// mustPeriod builds a period from two dates.
func mustPeriod(t *testing.T, from, to string) clock.Period {
	t.Helper()
	start, err := time.Parse("2006-01-02", from)
	if err != nil {
		t.Fatalf("date: %v", err)
	}
	end, err := time.Parse("2006-01-02", to)
	if err != nil {
		t.Fatalf("date: %v", err)
	}
	period, err := clock.New(start, end)
	if err != nil {
		t.Fatalf("period: %v", err)
	}
	return period
}
