// Package billing wires the billing gateway from its options: the configuration, the record store and
// its report projection, the token issuer, the policy, the trail, the tax table and the HTTP surface.
// It is the package a deployment and a test both open the service through, so the wiring exists once.
package billing

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"example.com/billing/internal/audit"
	"example.com/billing/internal/auth"
	"example.com/billing/internal/clock"
	"example.com/billing/internal/config"
	"example.com/billing/internal/httpapi"
	"example.com/billing/internal/ids"
	"example.com/billing/internal/money"
	"example.com/billing/internal/ratelimit"
	"example.com/billing/internal/reports"
	"example.com/billing/internal/store"
	"example.com/billing/internal/tax"
)

// Errors Open reports.
var (
	// ErrOptions is returned for an options value that cannot describe a deployment.
	ErrOptions = errors.New("billing: the options are not a usable deployment")
)

// Options are the values a deployment runs with. A zero value falls back to the configured default and
// the combination is validated before anything is built.
type Options struct {
	Addr           string
	Tenants        []string
	TokenSecret    string
	Currency       string
	CycleMonths    int
	GraceDays      int
	RetentionDays  int
	PageSize       int
	MaxInvoiceRows int
	MaxEvents      int
	MaxBodyBytes   int64
	SessionTTL     time.Duration
	TaxBasis       int
	AdminSubjects  []string
	// TaxRates are the rates the deployment applies. An empty list takes the default table.
	TaxRates []tax.Rate
	// Now pins the clock. A zero value runs on the system clock.
	Now time.Time
	// Seed loads the demonstration dataset, which is what a deployment started with --demo does.
	Seed bool
	// TrailSize is the number of audit records the trail keeps.
	TrailSize int
}

// Service is one open billing gateway.
type Service struct {
	cfg     config.Config
	store   *store.Store
	issuer  *auth.Issuer
	policy  *auth.Policy
	trail   *audit.Trail
	reports *reports.Registry
	tax     *tax.Table
	limiter *ratelimit.Limiter
	api     *httpapi.API
	mu      sync.RWMutex
	now     clock.Clock
}

// Open validates the options and builds the service. Every failure before the first record leaves the
// service unbuilt: a deployment that cannot sign a token does not answer a request.
func Open(opts Options) (*Service, error) {
	cfg := config.Default()
	cfg = cfg.Overlay(config.Config{
		Addr:           opts.Addr,
		Tenants:        opts.Tenants,
		TokenSecret:    opts.TokenSecret,
		Currency:       opts.Currency,
		CycleMonths:    opts.CycleMonths,
		GraceDays:      opts.GraceDays,
		RetentionDays:  opts.RetentionDays,
		MaxPageSize:    opts.PageSize,
		MaxInvoiceRows: opts.MaxInvoiceRows,
		MaxEvents:      opts.MaxEvents,
		MaxBodyBytes:   opts.MaxBodyBytes,
		SessionTTL:     opts.SessionTTL,
		TaxRateBasis:   opts.TaxBasis,
		AdminSubjects:  opts.AdminSubjects,
	})
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOptions, err)
	}
	now := clock.System()
	if !opts.Now.IsZero() {
		now = clock.Fixed(opts.Now)
	}
	st, err := store.Open(cfg.Limits(), now)
	if err != nil {
		return nil, fmt.Errorf("billing: %w", err)
	}
	issuer, err := auth.NewIssuer(cfg.TokenSecret, cfg.SessionTTL, now)
	if err != nil {
		return nil, fmt.Errorf("billing: %w", err)
	}
	registry, err := reports.NewDefaultRegistry()
	if err != nil {
		return nil, fmt.Errorf("billing: %w", err)
	}
	rates := opts.TaxRates
	if len(rates) == 0 {
		table, err := tax.DefaultTable(cfg.TaxRateBasis)
		if err != nil {
			return nil, fmt.Errorf("billing: %w", err)
		}
		rates = nil
		_ = table
	}
	table, err := taxTable(cfg.TaxRateBasis, opts.TaxRates)
	if err != nil {
		return nil, fmt.Errorf("billing: %w", err)
	}
	trailSize := opts.TrailSize
	if trailSize <= 0 {
		trailSize = 1024
	}
	service := &Service{
		cfg:     cfg,
		store:   st,
		issuer:  issuer,
		policy:  auth.DefaultPolicy(),
		trail:   audit.New(trailSize, now),
		reports: registry,
		tax:     table,
		limiter: ratelimit.New(cfg.MaxPageSize*4, time.Minute, now),
		now:     now,
	}
	if opts.Seed {
		if err := seedDemo(st, now); err != nil {
			return nil, fmt.Errorf("billing: seed: %w", err)
		}
	}
	api, err := httpapi.New(httpapi.Deps{
		Config: cfg, Store: st, Issuer: issuer, Policy: service.policy,
		Trail: service.trail, Reports: registry, Tax: table, Limiter: service.limiter, Now: now,
	})
	if err != nil {
		return nil, fmt.Errorf("billing: %w", err)
	}
	service.api = api
	return service, nil
}

// taxTable builds the rate table, falling back to the demonstration rates when the caller did not name
// any. A deployment that named its own rates gets exactly those.
func taxTable(basis int, rates []tax.Rate) (*tax.Table, error) {
	if len(rates) == 0 {
		return tax.DefaultTable(basis)
	}
	return tax.NewTable(basis, rates...)
}

// Handler returns the HTTP surface. A next handler is served every path the gateway does not own, so a
// deployment can mount the gateway beside its own routes.
func (s *Service) Handler(next http.Handler) http.Handler {
	if next == nil {
		return s.api
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, "/api/") {
			s.api.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SetClock pins every clock the service reads: the store's, the issuer's, the trail's, the limiter's
// and the surface's. A caller that moved one of them and not the others would have requests stamped
// with two different instants.
func (s *Service) SetClock(at time.Time) {
	fixed := clock.Fixed(at)
	s.mu.Lock()
	s.now = fixed
	s.mu.Unlock()
	s.store.SetClock(fixed)
	s.issuer.SetClock(fixed)
	s.trail.SetClock(fixed)
	s.limiter.SetClock(fixed)
	s.api.SetClock(fixed)
}

// Issue signs a token for a subject in a tenant with the scopes given.
func (s *Service) Issue(subject, tenant string, scopes []string, ttl time.Duration) (string, error) {
	if !s.cfg.HasTenant(tenant) {
		return "", fmt.Errorf("billing: this deployment does not serve tenant %q", tenant)
	}
	return s.issuer.Issue(subject, tenant, scopes, ttl)
}

// Verify checks a token and returns its claims.
func (s *Service) Verify(token string) (auth.Claims, error) { return s.issuer.Verify(token) }

// Store returns the record store.
func (s *Service) Store() *store.Store { return s.store }

// Config returns the validated configuration.
func (s *Service) Config() config.Config { return s.cfg }

// Trail returns the audit trail.
func (s *Service) Trail() *audit.Trail { return s.trail }

// Reports returns the report registry.
func (s *Service) Reports() *reports.Registry { return s.reports }

// Tax returns the tax table.
func (s *Service) Tax() *tax.Table { return s.tax }

// Policy returns the scope policy.
func (s *Service) Policy() *auth.Policy { return s.policy }

// Now returns the instant the service's clock reports.
func (s *Service) Now() time.Time {
	s.mu.RLock()
	now := s.now
	s.mu.RUnlock()
	return now().UTC()
}

// SeedDemo inserts the demonstration dataset a deployment starts with when it runs with --demo, and
// the one the tests build their expectations on. The identifiers are fixed so a test can name a
// record, and the instants are tied to the clock given.
func seedDemo(s *store.Store, now clock.Clock) error {
	at := clock.Day(now())
	tenants := []store.Tenant{
		{ID: "acme", Name: "Acme Logistics", Created: at.AddDate(0, -14, 0)},
		{ID: "globex", Name: "Globex Shipping", Created: at.AddDate(0, -8, 0)},
	}
	for _, tenant := range tenants {
		if err := s.PutTenant(tenant); err != nil {
			return err
		}
	}
	acme, globex := tenants[0].ID, tenants[1].ID
	if err := seedPlans(s, acme, globex); err != nil {
		return err
	}
	if err := seedAccounts(s, acme, globex, at); err != nil {
		return err
	}
	if err := seedSubscriptions(s, acme, globex, at); err != nil {
		return err
	}
	if err := seedEvents(s, acme, globex, at); err != nil {
		return err
	}
	return seedInvoices(s, acme, globex, at)
}

// seedPlans writes the price lists of the two demonstration tenants.
func seedPlans(s *store.Store, acme, globex string) error {
	seat, err := money.New(1000, "EUR")
	if err != nil {
		return err
	}
	base, err := money.New(4900, "EUR")
	if err != nil {
		return err
	}
	unit, err := money.New(2, "EUR")
	if err != nil {
		return err
	}
	plans := []store.Plan{
		{
			ID: "pln-000000000000000000000001", Tenant: acme, Code: "starter", Name: "Starter",
			BasePrice: base, SeatPrice: seat, UnitPrice: unit, IncludedSeats: 2,
			IncludedQuantity: 1000, Metric: "requests", Active: true,
		},
		{
			ID: "pln-000000000000000000000002", Tenant: acme, Code: "scale", Name: "Scale",
			BasePrice: mustAmount(19900), SeatPrice: mustAmount(1500), UnitPrice: mustAmount(1),
			IncludedSeats: 10, IncludedQuantity: 10000, Metric: "requests", Active: true,
		},
		{
			ID: "pln-000000000000000000000003", Tenant: globex, Code: "starter", Name: "Starter",
			BasePrice: mustAmount(3900), SeatPrice: mustAmount(900), UnitPrice: mustAmount(2),
			IncludedSeats: 1, IncludedQuantity: 500, Metric: "requests", Active: true,
		},
	}
	for _, plan := range plans {
		if err := s.PutPlan(plan); err != nil {
			return err
		}
	}
	return nil
}

// seedAccounts writes the accounts of the two demonstration tenants.
func seedAccounts(s *store.Store, acme, globex string, at time.Time) error {
	accounts := []store.Account{
		{
			ID: "acc-000000000000000000000001", Tenant: acme, Email: "ops@acme.example",
			Name: "Acme Operations", Created: at.AddDate(0, -13, 0),
		},
		{
			ID: "acc-000000000000000000000002", Tenant: acme, Email: "billing@acme.example",
			Name: "Acme Billing", Created: at.AddDate(0, -6, 0),
		},
		{
			ID: "acc-000000000000000000000003", Tenant: globex, Email: "ops@globex.example",
			Name: "Globex Operations", Created: at.AddDate(0, -7, 0),
		},
	}
	for _, account := range accounts {
		if err := s.PutAccount(account); err != nil {
			return err
		}
	}
	return nil
}

// seedSubscriptions writes one subscription per account, anchored on a day of the month that makes
// the current cycle easy to reason about.
func seedSubscriptions(s *store.Store, acme, globex string, at time.Time) error {
	anchor := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, time.UTC)
	subs := []store.Subscription{
		{
			ID: "sub-000000000000000000000001", Tenant: acme, Account: "acc-000000000000000000000001",
			Plan: "pln-000000000000000000000001", Seats: 5, Anchor: anchor,
			Status: store.StatusActive, UpdatedAt: at,
		},
		{
			ID: "sub-000000000000000000000002", Tenant: acme, Account: "acc-000000000000000000000002",
			Plan: "pln-000000000000000000000002", Seats: 12, Anchor: anchor,
			Status: store.StatusActive, UpdatedAt: at,
		},
		{
			ID: "sub-000000000000000000000003", Tenant: globex, Account: "acc-000000000000000000000003",
			Plan: "pln-000000000000000000000003", Seats: 4, Anchor: anchor,
			Status: store.StatusActive, UpdatedAt: at,
		},
	}
	for _, sub := range subs {
		if err := s.PutSubscription(sub); err != nil {
			return err
		}
	}
	return nil
}

// seedEvents writes the metered events of the demonstration tenants. Their statuses cover the three
// the report filter accepts, so the report has something to narrow whichever status a client sends.
func seedEvents(s *store.Store, acme, globex string, at time.Time) error {
	type seedEvent struct {
		tenant       string
		account      string
		subscription string
		quantity     int64
		day          int
		status       store.EventStatus
	}
	rows := []seedEvent{
		{acme, "acc-000000000000000000000001", "sub-000000000000000000000001", 1200, 2, store.EventRecorded},
		{acme, "acc-000000000000000000000001", "sub-000000000000000000000001", 800, 3, store.EventRecorded},
		{acme, "acc-000000000000000000000001", "sub-000000000000000000000001", 400, 4, store.EventBilled},
		{acme, "acc-000000000000000000000002", "sub-000000000000000000000002", 5400, 2, store.EventRecorded},
		{acme, "acc-000000000000000000000002", "sub-000000000000000000000002", 2600, 5, store.EventArchived},
		{globex, "acc-000000000000000000000003", "sub-000000000000000000000003", 300, 2, store.EventRecorded},
		{globex, "acc-000000000000000000000003", "sub-000000000000000000000003", 150, 3, store.EventArchived},
	}
	start := time.Date(at.Year(), at.Month(), 1, 9, 0, 0, 0, time.UTC)
	for _, row := range rows {
		event := store.UsageEvent{
			ID:           ids.New(ids.UsageEvent),
			Tenant:       row.tenant,
			Account:      row.account,
			Subscription: row.subscription,
			Metric:       "requests",
			Quantity:     row.quantity,
			At:           start.AddDate(0, 0, row.day-1),
			Status:       row.status,
		}
		// The event identifier is minted rather than written down: a test reads the events through
		// the API, so it never needs to name one.
		if err := s.AddEvent(event); err != nil {
			return err
		}
	}
	return nil
}

// seedInvoices writes one issued invoice and one paid invoice per demonstration tenant.
func seedInvoices(s *store.Store, acme, globex string, at time.Time) error {
	type seedInvoice struct {
		ID           string
		Tenant       string
		Account      string
		Subscription string
		Status       store.InvoiceStatus
		Total        int64
		Paid         int64
		PeriodStart  time.Time
		Days         int
	}
	start := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	rows := []seedInvoice{
		{
			ID: "inv-000000000000000000000001", Tenant: acme,
			Account: "acc-000000000000000000000001", Subscription: "sub-000000000000000000000001",
			Status: store.InvoiceOpen, Total: 7900, Paid: 0, PeriodStart: start, Days: 30,
		},
		{
			ID: "inv-000000000000000000000002", Tenant: acme,
			Account: "acc-000000000000000000000002", Subscription: "sub-000000000000000000000002",
			Status: store.InvoicePaid, Total: 22900, Paid: 22900, PeriodStart: start, Days: 30,
		},
		{
			ID: "inv-000000000000000000000003", Tenant: globex,
			Account: "acc-000000000000000000000003", Subscription: "sub-000000000000000000000003",
			Status: store.InvoiceOpen, Total: 6600, Paid: 0, PeriodStart: start, Days: 31,
		},
	}
	for _, row := range rows {
		total, err := money.New(row.Total, "EUR")
		if err != nil {
			return err
		}
		paid, err := money.New(row.Paid, "EUR")
		if err != nil {
			return err
		}
		period, err := clock.New(row.PeriodStart, row.PeriodStart.AddDate(0, 0, row.Days))
		if err != nil {
			return err
		}
		invoice := store.Invoice{
			ID: row.ID, Tenant: row.Tenant, Account: row.Account, Subscription: row.Subscription,
			Period: period, Issued: row.PeriodStart.AddDate(0, 0, row.Days),
			Due: row.PeriodStart.AddDate(0, 0, row.Days+14), Status: row.Status, Currency: "EUR",
			Lines: []store.Line{
				{
					ID: lineID(row.ID, 1), Kind: "base", Description: "Plan base price",
					Quantity: 1, UnitPrice: total, Amount: total,
				},
			},
			Total: total, Paid: paid,
		}
		if err := s.PutInvoice(invoice); err != nil {
			return err
		}
	}
	return nil
}

// lineID mints a deterministic line identifier from an invoice id, so the seed's invoices are the
// same on every run.
func lineID(invoice string, n int) string {
	suffix := invoice[len("inv-"):]
	return fmt.Sprintf("lin-%s%04d", suffix[:20], n)
}

// mustAmount returns an amount in EUR, for the seed's literals.
func mustAmount(minor int64) money.Amount {
	amount, err := money.New(minor, "EUR")
	if err != nil {
		panic(err)
	}
	return amount
}
