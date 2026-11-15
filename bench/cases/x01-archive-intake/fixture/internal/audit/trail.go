package audit

import (
	"example.com/archive/internal/clock"
	"example.com/archive/internal/idgen"
	"sync"
)

// Package audit is the archive's append-only trail: one entry per mutating handler and per refusal,
// readable only with the audit scope.

// Outcomes a trail entry records.
const (
	OutcomeOK       = "ok"
	OutcomeRefused  = "refused"
	OutcomeNotFound = "not-found"
	OutcomeFailed   = "failed"
)

// Entry is one audit record.
type Entry struct {
	ID      string `json:"id"`
	At      string `json:"at"`
	Subject string `json:"subject"`
	Tenant  string `json:"tenant"`
	Action  string `json:"action"`
	Target  string `json:"target"`
	Outcome string `json:"outcome"`
	Detail  string `json:"detail,omitempty"`
}

// Limit is how many entries the trail keeps. A bounded trail drops the oldest entry when it is full,
// which the operator page reports rather than hides.
type Limit int

// DefaultLimit is the trail size a deployment runs with: enough for a day of imports, small enough to
// stay in memory.
const DefaultLimit Limit = 5000

// Trail is the append-only trail of one process.
type Trail struct {
	mu      sync.Mutex
	now     clock.Clock
	limit   int
	entries []Entry
	dropped int
}

// New returns an empty trail keeping at most limit entries.
func New(limit Limit, now clock.Clock) *Trail {
	size := int(limit)
	if size <= 0 {
		size = int(DefaultLimit)
	}
	return &Trail{now: now, limit: size}
}

// Record appends one entry. The append never fails: a trail that cannot be written is not a reason to
// fail the request that already happened.
func (t *Trail) Record(subject, tenant, action, target, outcome, detail string) Entry {
	entry := Entry{
		ID:      idgen.New(idgen.Trail),
		At:      clock.Stamp(clock.Now(t.now)),
		Subject: subject,
		Tenant:  tenant,
		Action:  action,
		Target:  target,
		Outcome: outcome,
		Detail:  detail,
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = append(t.entries, entry)
	if len(t.entries) > t.limit {
		drop := len(t.entries) - t.limit
		t.entries = append([]Entry(nil), t.entries[drop:]...)
		t.dropped += drop
	}
	return entry
}

// Len is the number of entries the trail holds.
func (t *Trail) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.entries)
}

// Dropped is the number of entries the trail has forgotten at the front.
func (t *Trail) Dropped() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.dropped
}

// Entries returns a copy of the trail, oldest first.
func (t *Trail) Entries() []Entry {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Entry, len(t.entries))
	copy(out, t.entries)
	return out
}
