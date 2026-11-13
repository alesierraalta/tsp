// Package audit is the artifact server's bounded trail of mutating requests and refusals.
package audit

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"example.com/artifacts/internal/clock"
	"example.com/artifacts/internal/ids"
)

// Errors a record reports.
var (
	// ErrRecord is returned for a record that is not complete.
	ErrRecord = errors.New("audit: incomplete record")
)

// Outcomes a record can carry.
const (
	// OutcomeAccepted is a request the service performed.
	OutcomeAccepted = "accepted"
	// OutcomeRefused is a request the service refused.
	OutcomeRefused = "refused"
	// OutcomeFailed is a request the service attempted and could not finish.
	OutcomeFailed = "failed"
)

// Record is one entry of the trail.
type Record struct {
	ID      string    `json:"id"`
	At      time.Time `json:"at"`
	Project string    `json:"project"`
	Subject string    `json:"subject"`
	Action  string    `json:"action"`
	Target  string    `json:"target,omitempty"`
	Outcome string    `json:"outcome"`
	Detail  string    `json:"detail,omitempty"`
}

// Validate checks the record.
func (r Record) Validate() error {
	if !ids.ValidKind(r.ID, ids.Audit) {
		return fmt.Errorf("%w: id %q", ErrRecord, r.ID)
	}
	if r.Subject == "" {
		return fmt.Errorf("%w: no subject", ErrRecord)
	}
	if r.Action == "" {
		return fmt.Errorf("%w: no action", ErrRecord)
	}
	switch r.Outcome {
	case OutcomeAccepted, OutcomeRefused, OutcomeFailed:
	default:
		return fmt.Errorf("%w: outcome %q", ErrRecord, r.Outcome)
	}
	if r.At.IsZero() || r.At.Location() != time.UTC {
		return fmt.Errorf("%w: instant %s", ErrRecord, clock.Format(r.At))
	}
	return nil
}

// String renders the record for a console line.
func (r Record) String() string {
	detail := r.Detail
	if detail == "" {
		detail = "-"
	}
	return fmt.Sprintf("%s %s %s %s %s %s", clock.Format(r.At), r.Project, r.Subject, r.Action, r.Outcome, detail)
}

// Trail is a bounded ring of records.
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

// SetClock replaces the trail's clock.
func (t *Trail) SetClock(now clock.Clock) {
	if now == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.now = now
}

// Append writes one record.
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

// OfProject returns the records of one project.
func (t *Trail) OfProject(project string) []Record {
	all := t.All()
	out := all[:0]
	for _, record := range all {
		if record.Project == project {
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

// Dropped returns the number of records the bound removed.
func (t *Trail) Dropped() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.dropped
}

// Summary renders the trail's state for the console page.
func (t *Trail) Summary() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.dropped == 0 {
		return fmt.Sprintf("%d records, none dropped", len(t.records))
	}
	return fmt.Sprintf("%d records, %d dropped at the bound of %d", len(t.records), t.dropped, t.size)
}
