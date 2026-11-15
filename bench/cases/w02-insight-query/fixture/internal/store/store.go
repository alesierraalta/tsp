// Package store owns the tables the reports read and the code that puts rows in them.
package store

import (
	"fmt"

	"example.com/insight/internal/sqlmem"
)

// Tables the service ships with, and their columns. A report names a table and one of its columns;
// nothing else is addressable.
const (
	TableEvents   = "events"
	TableAccounts = "accounts"
	TableQuotas   = "quotas"
)

// Events columns.
var eventColumns = []string{"id", "tenant", "account", "status", "amount", "created"}

// Accounts columns.
var accountColumns = []string{"subject", "tenant", "display", "role"}

// Quotas columns.
var quotaColumns = []string{"tenant", "account", "limit", "used"}

// Store wraps the statement engine with the schema of the service.
type Store struct {
	db *sqlmem.DB
}

// Open returns a store with the schema declared and no rows.
func Open() *Store {
	db := sqlmem.Open()
	db.Define(TableEvents, eventColumns)
	db.Define(TableAccounts, accountColumns)
	db.Define(TableQuotas, quotaColumns)
	return &Store{db: db}
}

// DB is the engine, for the reports that run statements.
func (s *Store) DB() *sqlmem.DB { return s.db }

// Columns returns the column list of a table, or nil for a table the store does not carry.
func (s *Store) Columns(table string) []string {
	switch table {
	case TableEvents:
		return eventColumns
	case TableAccounts:
		return accountColumns
	case TableQuotas:
		return quotaColumns
	default:
		return nil
	}
}

// Tables lists the tables the store carries.
func (s *Store) Tables() []string {
	return []string{TableEvents, TableAccounts, TableQuotas}
}

// Event is one metered event: an amount booked for an account in a tenant.
type Event struct {
	ID      int64
	Tenant  string
	Account string
	Status  string
	Amount  int64
	Created string
}

// Statuses an event may carry. A report filters on them by name, so the list is the contract.
var Statuses = []string{"booked", "archived", "pending"}

// IsStatus reports whether status is one the store accepts.
func IsStatus(status string) bool {
	for _, s := range Statuses {
		if s == status {
			return true
		}
	}
	return false
}

// AddEvent appends one event.
func (s *Store) AddEvent(e Event) error {
	if !IsStatus(e.Status) {
		return fmt.Errorf("store: status %q is not accepted", e.Status)
	}
	if e.Tenant == "" || e.Account == "" {
		return fmt.Errorf("store: an event needs a tenant and an account")
	}
	if e.Amount < 0 {
		return fmt.Errorf("store: amount %d is negative", e.Amount)
	}
	return s.db.Insert(TableEvents, map[string]sqlmem.Value{
		"id":      sqlmem.Int(e.ID),
		"tenant":  sqlmem.Text(e.Tenant),
		"account": sqlmem.Text(e.Account),
		"status":  sqlmem.Text(e.Status),
		"amount":  sqlmem.Int(e.Amount),
		"created": sqlmem.Text(e.Created),
	})
}

// Account is one account the directory knows.
type Account struct {
	Subject string
	Tenant  string
	Display string
	Role    string
}

// AddAccount appends one account row.
func (s *Store) AddAccount(a Account) error {
	if a.Subject == "" || a.Tenant == "" {
		return fmt.Errorf("store: an account needs a subject and a tenant")
	}
	return s.db.Insert(TableAccounts, map[string]sqlmem.Value{
		"subject": sqlmem.Text(a.Subject),
		"tenant":  sqlmem.Text(a.Tenant),
		"display": sqlmem.Text(a.Display),
		"role":    sqlmem.Text(a.Role),
	})
}

// Quota is the per-account ceiling of one tenant.
type Quota struct {
	Tenant  string
	Account string
	Limit   int64
	Used    int64
}

// AddQuota appends one quota row.
func (s *Store) AddQuota(q Quota) error {
	if q.Limit <= 0 {
		return fmt.Errorf("store: limit %d is not positive", q.Limit)
	}
	return s.db.Insert(TableQuotas, map[string]sqlmem.Value{
		"tenant":  sqlmem.Text(q.Tenant),
		"account": sqlmem.Text(q.Account),
		"limit":   sqlmem.Int(q.Limit),
		"used":    sqlmem.Int(q.Used),
	})
}
