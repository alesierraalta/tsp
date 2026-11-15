package audit

import (
	"fmt"
	"sync"
	"time"

	"example.com/billing/internal/clock"
	"example.com/billing/internal/ids"
)

// Trail is a bounded ring of records. A process that appended without a bound would grow until it was
// restarted, and a restart loses the trail anyway, so the bound is stated: the newest records are the
// ones an operator can read.
type Trail struct {
	mu      sync.RWMutex
	now     clock.Clock
	records []Record
	size    int
	dropped int
}

// New returns a trail that keeps at most size records.
func New(size int, now clock.Clock) *Trail {
	if size <= 0 {
		size = 512
	}
	if now == nil {
		now = clock.System()
	}
	return &Trail{now: now, size: size}
}

// SetClock replaces the clock the trail stamps records with.
func (t *Trail) SetClock(now clock.Clock) {
	if now == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.now = now
}

// Append writes one record and returns it with its identifier and instant filled in.
func (t *Trail) Append(record Record) (Record, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if record.ID == "" {
		record.ID = ids.New(ids.Audit)
	}
	if record.At.IsZero() {
		record.At = t.now().UTC()
	}
	if err := record.Validate(); err != nil {
		return Record{}, err
	}
	if len(t.records) >= t.size {
		copy(t.records, t.records[1:])
		t.records = t.records[:len(t.records)-1]
		t.dropped++
	}
	t.records = append(t.records, record)
	return record, nil
}

// All returns the records oldest first.
func (t *Trail) All() []Record {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return append([]Record(nil), t.records...)
}

// OfTenant returns the records of one tenant, oldest first.
func (t *Trail) OfTenant(tenant string) []Record {
	all := t.All()
	out := all[:0]
	for _, record := range all {
		if record.Tenant == tenant {
			out = append(out, record)
		}
	}
	return out
}

// Since returns the records written at or after an instant.
func (t *Trail) Since(at time.Time) []Record {
	all := t.All()
	out := all[:0]
	for _, record := range all {
		if !record.At.Before(at) {
			out = append(out, record)
		}
	}
	return out
}

// Len returns the number of records the trail carries.
func (t *Trail) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.records)
}

// Dropped returns the number of records the bound removed, which the console reports so a trail that
// lost entries says so instead of looking complete.
func (t *Trail) Dropped() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.dropped
}

// Summary is what the console prints about the trail.
func (t *Trail) Summary() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.dropped == 0 {
		return fmt.Sprintf("%d records, none dropped", len(t.records))
	}
	return fmt.Sprintf("%d records, %d dropped at the bound of %d", len(t.records), t.dropped, t.size)
}

// Accepted records a request the service performed.
func (t *Trail) Accepted(tenant, subject, action, target, detail string) (Record, error) {
	return t.Append(Record{
		Tenant: tenant, Subject: subject, Action: action, Target: target,
		Outcome: OutcomeAccepted, Detail: detail,
	})
}

// Refused records a request the service refused, with the reason the client never sees.
func (t *Trail) Refused(tenant, subject, action, target, detail string) (Record, error) {
	return t.Append(Record{
		Tenant: tenant, Subject: subject, Action: action, Target: target,
		Outcome: OutcomeRefused, Detail: detail,
	})
}

// Failed records a request the service attempted and could not finish.
func (t *Trail) Failed(tenant, subject, action, target, detail string) (Record, error) {
	return t.Append(Record{
		Tenant: tenant, Subject: subject, Action: action, Target: target,
		Outcome: OutcomeFailed, Detail: detail,
	})
}
