// Package store is the ledger's in-memory state: the events it has accepted and the statements it issued.
package store

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"example.com/ledgerclose/internal/calendar"
	"example.com/ledgerclose/internal/invoice"
	"example.com/ledgerclose/internal/query"
	"example.com/ledgerclose/internal/usage"
)

// Errors the store reports.
var (
	ErrDuplicate     = errors.New("store: duplicate id")
	ErrUnknown       = errors.New("store: unknown invoice")
	ErrUnknownColumn = errors.New("store: unknown column")
	ErrTooManyRows   = errors.New("store: row bound")
)

// The columns the usage relation carries: the names a report definition declares, and columnValue is the
// only place one of them becomes a field.
const (
	ColumnID       = "usage_id"
	ColumnAccount  = "usage_account"
	ColumnMetric   = "usage_metric"
	ColumnCurrency = "usage_currency"
	ColumnQuantity = "usage_quantity"
	ColumnOccurred = "usage_occurred_at"
)

// MaxRows bounds every read: a definition with a larger page cap still cannot ask for an unbounded
// walk.
const MaxRows = 5_000

// Store is the service state. It is safe for concurrent use.
type Store struct {
	mu       sync.RWMutex
	events   map[string]usage.Event
	invoices map[string]invoice.Invoice
}

// New returns an empty store.
func New() *Store {
	return &Store{events: make(map[string]usage.Event, 256), invoices: make(map[string]invoice.Invoice, 64)}
}

// PutEvents stores a batch. The whole batch is refused if any id is already stored: an ingest that
// half-applied would leave the sender a batch to reconcile by hand, and a re-sent event must not be
// billed twice.
func (s *Store) PutEvents(events []usage.Event) error {
	if len(events) > MaxRows {
		return fmt.Errorf("%w: %d events, cap %d", ErrTooManyRows, len(events), MaxRows)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, event := range events {
		if _, exists := s.events[event.ID]; exists {
			return fmt.Errorf("%w: event %q", ErrDuplicate, event.ID)
		}
	}
	for _, event := range events {
		s.events[event.ID] = event
	}
	return nil
}

// PutInvoice stores a statement. A statement of the same id replaces the earlier one: a close is
// re-runnable and the second run is the one that counts.
func (s *Store) PutInvoice(statement invoice.Invoice) error {
	if statement.ID == "" {
		return fmt.Errorf("%w: statement has no id", ErrUnknown)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invoices[statement.ID] = statement
	return nil
}

// Invoice returns one statement by id.
func (s *Store) Invoice(id string) (invoice.Invoice, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	statement, ok := s.invoices[id]
	if !ok {
		return invoice.Invoice{}, fmt.Errorf("%w: %q", ErrUnknown, id)
	}
	return statement, nil
}

// EventsByAccount returns the events of one account in one period, oldest first. The period is
// applied here as well as in the assembly, so a close can never bill an event another month owns.
// The account id comes from the directory's tenant list: an event does not carry a tenant.
func (s *Store) EventsByAccount(account string, period calendar.Period, limit int) []usage.Event {
	if limit <= 0 || limit > MaxRows {
		limit = MaxRows
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]usage.Event, 0, 64)
	for _, event := range s.events {
		if event.Account != account || (period.Valid() && !period.Contains(event.OccurredAt.UTC())) {
			continue
		}
		out = append(out, event)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OccurredAt.Equal(out[j].OccurredAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].OccurredAt.Before(out[j].OccurredAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// SelectEvents applies a selection a report builder produced. A clause names a column the definition
// declared; matches is the second check, and a column it does not carry is refused rather than ignored.
// Values are compared as values: an integer clause against an integer and an instant against an instant,
// so a string that looks like a timestamp filters nothing. The caller's account scope is applied before the
// page is cut, and a read with no scope reads nothing.
func (s *Store) SelectEvents(criteria query.Criteria) ([]usage.Event, error) {
	for _, order := range criteria.Sorts {
		if _, err := columnValue(usage.Event{}, order.Column); err != nil {
			return nil, err
		}
	}
	scope := make(map[string]bool, len(criteria.Accounts))
	for _, account := range criteria.Accounts {
		scope[account] = true
	}
	if len(scope) == 0 {
		return nil, nil
	}
	limit := criteria.Limit
	if limit <= 0 || limit > MaxRows {
		limit = MaxRows
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	matched := make([]usage.Event, 0, 64)
	for _, event := range s.events {
		if !scope[event.Account] {
			continue
		}
		ok, err := matches(event, criteria.Clauses)
		if err != nil {
			return nil, err
		}
		if ok {
			matched = append(matched, event)
		}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		for _, order := range criteria.Sorts {
			less, equal := compare(matched[i], matched[j], order.Column)
			if !equal {
				if order.Desc {
					return !less
				}
				return less
			}
		}
		return matched[i].ID < matched[j].ID
	})
	if len(matched) > limit {
		matched = matched[:limit]
	}
	return matched, nil
}

// columnValue reads one column of an event through the allowlist.
func columnValue(event usage.Event, column string) (any, error) {
	switch column {
	case ColumnID:
		return event.ID, nil
	case ColumnAccount:
		return event.Account, nil
	case ColumnMetric:
		return event.Metric, nil
	case ColumnCurrency:
		return event.Currency, nil
	case ColumnQuantity:
		return event.Quantity, nil
	case ColumnOccurred:
		return event.OccurredAt.UTC(), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownColumn, column)
	}
}

// matches applies the clauses to one event.
func matches(event usage.Event, clauses []query.Clause) (bool, error) {
	for _, clause := range clauses {
		actual, err := columnValue(event, clause.Column)
		if err != nil {
			return false, err
		}
		switch want := clause.Value.(type) {
		case string:
			if actual.(string) != want {
				return false, nil
			}
		case int64:
			if actual.(int64) != want {
				return false, nil
			}
		case time.Time:
			if !actual.(time.Time).Equal(want) {
				return false, nil
			}
		default:
			return false, fmt.Errorf("%w: %q", ErrUnknownColumn, clause.Column)
		}
	}
	return true, nil
}

// compare orders two events by one column and reports whether they are equal on it.
func compare(a, b usage.Event, column string) (less, equal bool) {
	left, err := columnValue(a, column)
	if err != nil {
		return false, true
	}
	right, _ := columnValue(b, column)
	switch value := left.(type) {
	case string:
		other := right.(string)
		return value < other, value == other
	case int64:
		other := right.(int64)
		return value < other, value == other
	case time.Time:
		other := right.(time.Time)
		return value.Before(other), value.Equal(other)
	default:
		return false, true
	}
}
