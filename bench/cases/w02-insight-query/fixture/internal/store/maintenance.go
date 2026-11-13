package store

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Maintenance is the operator surface of the store: the row counts, the last load, and the reset a
// development instance runs between experiments.
type Maintenance struct {
	mu     sync.Mutex
	loaded time.Time
	loads  int
	store  *Store
	now    func() time.Time
}

// NewMaintenance returns a maintenance handle over a store.
func NewMaintenance(s *Store) *Maintenance {
	return &Maintenance{store: s, now: func() time.Time { return time.Now().UTC() }}
}

// SetClock replaces the clock the handle stamps loads with.
func (m *Maintenance) SetClock(now func() time.Time) {
	if now == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = now
}

// Loaded records that a load finished.
func (m *Maintenance) Loaded() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loaded = m.now()
	m.loads++
}

// Loads reports how many loads the instance has seen.
func (m *Maintenance) Loads() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loads
}

// LoadedAt reports when the last load finished, or the zero time.
func (m *Maintenance) LoadedAt() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loaded
}

// Counts reports how many rows each table carries, keyed by table name.
func (m *Maintenance) Counts() map[string]int {
	counts := make(map[string]int, len(m.store.Tables()))
	for _, table := range m.store.Tables() {
		counts[table] = m.store.DB().Count(table)
	}
	return counts
}

// Tables lists the tables in the order the store declares them.
func (m *Maintenance) Tables() []string { return m.store.Tables() }

// Describe renders the store's shape as the operator endpoint serves it.
func (m *Maintenance) Describe() map[string]any {
	counts := m.Counts()
	rows := 0
	for _, n := range counts {
		rows += n
	}
	loaded := ""
	if at := m.LoadedAt(); !at.IsZero() {
		loaded = at.Format(time.RFC3339)
	}
	return map[string]any{
		"tables":    m.Tables(),
		"counts":    counts,
		"rows":      rows,
		"loads":     m.Loads(),
		"loaded_at": loaded,
	}
}

// SortedTables lists the tables sorted, which is the order an operator report prints.
func SortedTables(tables []string) []string {
	out := append([]string(nil), tables...)
	sort.Strings(out)
	return out
}

// Verify checks that every table the store declares has its columns declared too. It is the check a
// deployment runs at start-up, and it refuses a store whose schema is half built.
func (s *Store) Verify() error {
	for _, table := range s.Tables() {
		columns := s.Columns(table)
		if len(columns) == 0 {
			return fmt.Errorf("store: table %q has no columns", table)
		}
		seen := map[string]bool{}
		for _, column := range columns {
			if column == "" {
				return fmt.Errorf("store: table %q declares an empty column", table)
			}
			if seen[column] {
				return fmt.Errorf("store: table %q declares %q twice", table, column)
			}
			seen[column] = true
		}
	}
	return nil
}
