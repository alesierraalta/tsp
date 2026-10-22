package store

import (
	"sync"
	"time"
)

// Store is shared by gateways serving the same quota namespace.
// All operations in a transaction observe one clock reading.
type Store struct {
	mu     sync.Mutex
	now    func() time.Time
	rows   map[string]*row
	serial uint64
}

type row struct {
	count   int
	expires time.Time
	slots   map[uint64]time.Time
}

type Tx struct {
	row   *row
	now   time.Time
	store *Store
}

func New(now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{now: now, rows: make(map[string]*row)}
}

func (s *Store) Transaction(key string, fn func(*Tx)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	r := s.rows[key]
	if r == nil || (!r.expires.IsZero() && !now.Before(r.expires)) {
		r = &row{slots: make(map[uint64]time.Time)}
		s.rows[key] = r
	}
	for id, deadline := range r.slots {
		if !now.Before(deadline) {
			delete(r.slots, id)
			if r.count > 0 {
				r.count--
			}
		}
	}
	fn(&Tx{row: r, now: now, store: s})
	if r.count == 0 && len(r.slots) == 0 {
		delete(s.rows, key)
	}
}

func (t *Tx) Increment(lease time.Duration) (uint64, int) {
	t.store.serial++
	id := t.store.serial
	t.row.slots[id] = t.now.Add(lease)
	t.row.count++
	return id, t.row.count
}

func (t *Tx) ExpireAfter(lease time.Duration) {
	deadline := t.now.Add(lease)
	if deadline.After(t.row.expires) {
		t.row.expires = deadline
	}
}

func (t *Tx) Remove(id uint64) {
	delete(t.row.slots, id)
	if t.row.count > 0 {
		t.row.count--
	}
}

func (t *Tx) Owns(id uint64) bool {
	deadline, ok := t.row.slots[id]
	return ok && t.now.Before(deadline)
}

func (t *Tx) Count() int     { return t.row.count }
func (t *Tx) Now() time.Time { return t.now }
