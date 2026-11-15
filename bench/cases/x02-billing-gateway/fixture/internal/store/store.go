package store

import (
	"fmt"
	"sync"
	"time"

	"example.com/billing/internal/clock"
	"example.com/billing/internal/config"
	"example.com/billing/internal/query"
	"example.com/billing/internal/sqlrun"
)

// Store is the in-memory record set with the report projection built over it. Every read of a report
// goes through the projection, and every write refreshes it, so a report answers the records the
// service currently carries. The writers that take a usage event or an invoice live here, beside the
// projection they refresh.
type Store struct {
	mu            sync.RWMutex
	db            *sqlrun.DB
	limits        config.Limits
	now           clock.Clock
	tenants       map[string]Tenant
	plans         map[string]Plan
	accounts      map[string]Account
	subscriptions map[string]Subscription
	invoices      map[string]Invoice
	events        map[string]UsageEvent
	credits       map[string]Credit
	payments      map[string]Payment
}

// Open returns an empty store. The caps are validated here, because a store built with a page size of
// zero would answer every listing with no rows and nobody would notice until a customer did.
func Open(limits config.Limits, now clock.Clock) (*Store, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	if now == nil {
		return nil, fmt.Errorf("store: a clock is required")
	}
	s := &Store{
		db:            sqlrun.New(),
		limits:        limits,
		now:           now,
		tenants:       make(map[string]Tenant),
		plans:         make(map[string]Plan),
		accounts:      make(map[string]Account),
		subscriptions: make(map[string]Subscription),
		invoices:      make(map[string]Invoice),
		events:        make(map[string]UsageEvent),
		credits:       make(map[string]Credit),
		payments:      make(map[string]Payment),
	}
	if err := s.createTables(); err != nil {
		return nil, err
	}
	return s, nil
}

// SetClock replaces the clock the store stamps records with.
func (s *Store) SetClock(now clock.Clock) {
	if now == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

// Now reports the store's clock reading.
func (s *Store) Now() time.Time {
	s.mu.RLock()
	now := s.now
	s.mu.RUnlock()
	return now()
}

// Limits returns the caps this store enforces.
func (s *Store) Limits() config.Limits {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.limits
}

// Query answers one statement over the projection.
func (s *Store) Query(stmt query.Stmt) (sqlrun.Result, error) {
	return s.db.Query(stmt)
}

// QueryText answers one statement written as text, which is the path a statement assembled outside
// the builder takes.
func (s *Store) QueryText(text string, args ...any) (sqlrun.Result, error) {
	return s.db.QueryText(text, args...)
}

// Tables lists the projection's tables.
func (s *Store) Tables() []string { return s.db.Tables() }

// Count reports the number of rows in a projection table.
func (s *Store) Count(table string) int { return s.db.Count(table) }

// Tenant returns one tenant.
func (s *Store) Tenant(id string) (Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tenant, ok := s.tenants[id]
	if !ok {
		return Tenant{}, fmt.Errorf("%w: tenant %q", ErrNotFound, id)
	}
	return tenant, nil
}

// Tenants returns the tenants in id order.
func (s *Store) Tenants() []Tenant {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return sortedByID(s.tenants, func(t Tenant) string { return t.ID })
}

// PutTenant stores one tenant.
func (s *Store) PutTenant(tenant Tenant) error {
	if err := tenant.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tenants[tenant.ID]; exists {
		return fmt.Errorf("%w: tenant %q", ErrDuplicate, tenant.ID)
	}
	s.tenants[tenant.ID] = tenant
	return nil
}

// ensureTenant refuses a record that names a tenant the store does not carry, which is what keeps a
// plan or an account from existing in a tenant nobody can read.
func (s *Store) ensureTenant(id string) error {
	if _, ok := s.tenants[id]; !ok {
		return fmt.Errorf("%w: tenant %q", ErrNotFound, id)
	}
	return nil
}

// textValue and numberValue are declared in records.go; these helpers keep the projection code short.
func (s *Store) insertEventRow(event UsageEvent) error {
	return s.db.Insert(tableUsage,
		textValue(event.ID), textValue(event.Tenant), textValue(event.Account),
		textValue(event.Subscription), textValue(event.Metric), numberValue(event.Quantity),
		textValue(string(event.Status)), numberValue(clock.DayNumber(event.At)),
	)
}

// AddEvent records one usage event. The store refuses a write past the deployment's event cap
// instead of dropping the oldest event: an event that disappears is a quantity nobody bills, and a
// refusal is something the reporter can act on. The projection is refreshed before the call returns.
func (s *Store) AddEvent(event UsageEvent) error {
	if err := event.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureTenant(event.Tenant); err != nil {
		return err
	}
	account, ok := s.accounts[event.Account]
	if !ok || account.Tenant != event.Tenant {
		return fmt.Errorf("%w: account %q in tenant %q", ErrNotFound, event.Account, event.Tenant)
	}
	if _, exists := s.events[event.ID]; exists {
		return fmt.Errorf("%w: event %q", ErrDuplicate, event.ID)
	}
	if sub, ok := s.subscriptions[event.Subscription]; ok && sub.Account != event.Account {
		return fmt.Errorf("%w: event %q names subscription %q of another account", ErrInvalid, event.ID, event.Subscription)
	}
	if len(s.events) >= s.limits.EventCount {
		return fmt.Errorf("%w: %d usage events", ErrCap, s.limits.EventCount)
	}
	s.events[event.ID] = event
	return s.refreshUsage()
}

// PutInvoice stores one invoice and refreshes the projection. An invoice whose lines do not add to
// its total, and one that carries more lines than the deployment accepts, are refused here.
func (s *Store) PutInvoice(invoice Invoice) error {
	if err := invoice.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureTenant(invoice.Tenant); err != nil {
		return err
	}
	if _, exists := s.invoices[invoice.ID]; exists {
		return fmt.Errorf("%w: invoice %q", ErrDuplicate, invoice.ID)
	}
	if len(invoice.Lines) > s.limits.InvoiceRows {
		return fmt.Errorf("%w: invoice %q carries %d lines of %d", ErrCap, invoice.ID, len(invoice.Lines), s.limits.InvoiceRows)
	}
	s.invoices[invoice.ID] = invoice
	return s.refreshInvoices()
}
