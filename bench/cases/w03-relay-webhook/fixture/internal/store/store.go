// Package store keeps the records the pipeline imports and the totals the reports read.
package store

import (
	"fmt"
	"sort"
	"sync"

	"example.com/relay/internal/wire"
)

// Store is the in-memory record store. It is safe for concurrent use.
type Store struct {
	mu      sync.RWMutex
	records map[string]wire.Record
	order   []string
}

// New returns an empty store.
func New() *Store {
	return &Store{records: map[string]wire.Record{}}
}

// Upsert stores one record. A record with the same id replaces the earlier one and keeps its place
// in the insertion order, so a redelivered record does not reorder the report.
func (s *Store) Upsert(record wire.Record) error {
	if err := record.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.records[record.ID]; !ok {
		s.order = append(s.order, record.ID)
	}
	s.records[record.ID] = record
	return nil
}

// Get returns one record.
func (s *Store) Get(id string) (wire.Record, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[id]
	return record, ok
}

// List returns the records in insertion order.
func (s *Store) List() []wire.Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]wire.Record, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.records[id])
	}
	return out
}

// OfKind returns the records of one kind, in insertion order.
func (s *Store) OfKind(kind string) []wire.Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []wire.Record
	for _, id := range s.order {
		if s.records[id].Kind == kind {
			out = append(out, s.records[id])
		}
	}
	return out
}

// Count reports how many records the store carries.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.order)
}

// Total sums the amount of every record, in cents. The sum is integer arithmetic: the ledger has no
// fractional amounts, and a float total would round a column of cents.
func (s *Store) Total() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var total int64
	for _, record := range s.records {
		total += record.Amount
	}
	return total
}

// TotalsByKind sums the amount of each kind, in cents.
func (s *Store) TotalsByKind() map[string]int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string]int64{}
	for _, record := range s.records {
		out[record.Kind] += record.Amount
	}
	return out
}

// Kinds lists the kinds the store carries, sorted.
func (s *Store) Kinds() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	for _, record := range s.records {
		seen[record.Kind] = true
	}
	out := make([]string, 0, len(seen))
	for kind := range seen {
		out = append(out, kind)
	}
	sort.Strings(out)
	return out
}

// Split divides a total into parts shares whose sum is exactly the total; every share differs by at
// most one cent from every other.
func Split(total int64, parts int) ([]int64, error) {
	if parts <= 0 {
		return nil, fmt.Errorf("store: %d parts", parts)
	}
	shares := make([]int64, 0, parts)
	base := total / int64(parts)
	remainder := total % int64(parts)
	if remainder < 0 {
		base--
		remainder += int64(parts)
	}
	for i := 0; i < parts; i++ {
		share := base
		if int64(i) < remainder {
			share++
		}
		shares = append(shares, share)
	}
	return shares, nil
}
