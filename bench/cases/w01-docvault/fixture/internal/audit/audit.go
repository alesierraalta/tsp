// Package audit keeps the append-only trail of the operations the service performed.
package audit

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Entry is one audit record. The trail is what an operator reads after an incident, so every field
// is a value and never a pointer into the request that produced it.
type Entry struct {
	Sequence int64     `json:"sequence"`
	At       time.Time `json:"at"`
	Acting   string    `json:"acting"`
	Tenant   string    `json:"tenant"`
	Action   string    `json:"action"`
	Object   string    `json:"object"`
	Outcome  string    `json:"outcome"`
	Detail   string    `json:"detail"`
}

// Outcomes an entry may carry.
const (
	OutcomeOK      = "ok"
	OutcomeRefused = "refused"
	OutcomeFailed  = "failed"
)

// Trail is an in-memory append-only log.
type Trail struct {
	mu      sync.Mutex
	entries []Entry
	seq     int64
	now     func() time.Time
}

// New returns an empty trail.
func New() *Trail {
	return &Trail{now: func() time.Time { return time.Now().UTC() }}
}

// SetClock replaces the clock the trail stamps entries with.
func (t *Trail) SetClock(now func() time.Time) {
	if now == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.now = now
}

// Record appends one entry and returns it with its sequence number.
func (t *Trail) Record(acting, tenant, action, object, outcome, detail string) Entry {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seq++
	entry := Entry{
		Sequence: t.seq,
		At:       t.now(),
		Acting:   acting,
		Tenant:   tenant,
		Action:   action,
		Object:   object,
		Outcome:  outcome,
		Detail:   detail,
	}
	t.entries = append(t.entries, entry)
	return entry
}

// Since returns the entries after a sequence number, oldest first.
func (t *Trail) Since(sequence int64) []Entry {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []Entry
	for _, e := range t.entries {
		if e.Sequence > sequence {
			out = append(out, e)
		}
	}
	return out
}

// OfTenant returns the entries of one tenant, newest first, at most limit of them.
func (t *Trail) OfTenant(tenant string, limit int) []Entry {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []Entry
	for _, e := range t.entries {
		if e.Tenant == tenant {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sequence > out[j].Sequence })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Len reports how many entries the trail carries.
func (t *Trail) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.entries)
}

// Reset drops every entry. Only the administrative purge route calls it.
func (t *Trail) Reset(acting, tenant string) {
	t.mu.Lock()
	n := len(t.entries)
	t.entries = nil
	t.mu.Unlock()
	t.Record(acting, tenant, "audit:purge", "", OutcomeOK, fmt.Sprintf("dropped %d entries", n))
}
