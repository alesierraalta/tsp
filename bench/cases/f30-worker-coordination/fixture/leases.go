package workqueue

import (
	"sync"
	"time"
)

type Lease struct {
	Key        string
	Owner      string
	Generation uint64
	Expires    time.Time
}

type LeaseBook struct {
	mu   sync.Mutex
	rows map[string]Lease
	next uint64
}

func NewLeaseBook() *LeaseBook {
	return &LeaseBook{rows: make(map[string]Lease)}
}

func (b *LeaseBook) Acquire(key, owner string, now time.Time, ttl time.Duration) (Lease, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if row, ok := b.rows[key]; ok && now.Before(row.Expires) {
		return Lease{}, false
	}
	b.next++
	row := Lease{Key: key, Owner: owner, Generation: b.next, Expires: now.Add(ttl)}
	b.rows[key] = row
	return row, true
}

func (b *LeaseBook) Current(key string) (Lease, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	row, ok := b.rows[key]
	return row, ok
}

func (b *LeaseBook) Release(key string, generation uint64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	row, ok := b.rows[key]
	if !ok || row.Generation != generation {
		return false
	}
	delete(b.rows, key)
	return true
}

func (b *LeaseBook) Count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.rows)
}
