// Package audit is the append-only trail of what the service did: an ingest, an assignment, a close, an
// export, a delivery and every refusal a mutating route answered. Each entry gets the next sequence
// number, and the trail refuses to grow past its cap rather than dropping the oldest entry: a dropped
// entry is a gap nobody can reconstruct, and an audit trail with a hole is worse than one that stopped.
package audit

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrFull is reported when the trail is at its cap.
var ErrFull = errors.New("audit: trail is full")

// Entry is one recorded action.
type Entry struct {
	Seq    int64
	Kind   string
	Actor  string
	Detail string
	At     time.Time
}

// Trail is the append-only log.
type Trail struct {
	mu      sync.Mutex
	entries []Entry
	max     int
	next    int64
}

// New returns an empty trail capped at max entries. The cap is inside 1..1,000,000: a deployment cannot
// be configured to keep a trail a process cannot hold.
func New(max int) (*Trail, error) {
	if max < 1 || max > 1000000 {
		return nil, fmt.Errorf("audit: cap %d", max)
	}
	return &Trail{entries: make([]Entry, 0, 64), max: max, next: 1}, nil
}

// Append records one entry and returns it. When the trail is full the entry is refused with ErrFull and
// the trail is unchanged: the sequence number is not consumed, so a caller that retries after an
// operator raises the cap gets the same next number.
func (t *Trail) Append(kind, actor, detail string, at time.Time) (Entry, error) {
	if kind == "" {
		return Entry{}, fmt.Errorf("audit: entry has no kind")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.entries) >= t.max {
		return Entry{}, fmt.Errorf("%w: %d entries", ErrFull, len(t.entries))
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	entry := Entry{Seq: t.next, Kind: kind, Actor: actor, Detail: detail, At: at.UTC()}
	t.next++
	t.entries = append(t.entries, entry)
	return entry, nil
}

// Len reports how many entries the trail holds.
func (t *Trail) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.entries)
}

// Entries returns the newest entries first, up to limit. A limit of zero or less returns every entry, and
// a limit above the trail's length returns the trail.
func (t *Trail) Entries(limit int) []Entry {
	t.mu.Lock()
	defer t.mu.Unlock()
	count := len(t.entries)
	if limit > 0 && limit < count {
		count = limit
	}
	result := make([]Entry, 0, count)
	for i := len(t.entries) - 1; i >= 0 && len(result) < count; i-- {
		result = append(result, t.entries[i])
	}
	return result
}

// Since returns the entries with a sequence number above seq, oldest first, up to limit.
func (t *Trail) Since(seq int64, limit int) []Entry {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]Entry, 0, 32)
	for _, entry := range t.entries {
		if entry.Seq <= seq {
			continue
		}
		if limit > 0 && len(result) >= limit {
			break
		}
		result = append(result, entry)
	}
	return result
}

// Kinds counts the entries by kind, for the operator console.
func (t *Trail) Kinds() map[string]int {
	t.mu.Lock()
	defer t.mu.Unlock()
	counts := map[string]int{}
	for _, entry := range t.entries {
		counts[entry.Kind]++
	}
	return counts
}
