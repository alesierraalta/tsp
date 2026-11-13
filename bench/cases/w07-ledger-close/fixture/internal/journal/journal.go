// Package journal is the append-only record of what the service did: every ingest, close and delivery
// appends one entry, and the sequence is what an operator reconciles against.
package journal

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sync"
	"time"
)

// Errors the journal reports.
var (
	ErrInvalid   = errors.New("journal: invalid entry")
	ErrFull      = errors.New("journal: no room for another entry")
	ErrExhausted = errors.New("journal: sequence is exhausted")
)

// MaxEntries bounds the trail. A deployment that needs more runs an archive step: the service refuses to grow
// without one instead of dropping the oldest entries, because a dropped entry is a gap nobody can reconstruct.
const MaxEntries = 100_000

// refRE is the form of a reference: a statement id, a batch id, a delivery. It is written to the export, so
// it carries no control character and no comma.
var refRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`)

// Entry is one recorded action.
type Entry struct {
	Seq    int64     `json:"seq"`
	At     time.Time `json:"at"`
	Actor  string    `json:"actor"`
	Tenant string    `json:"tenant"`
	Action string    `json:"action"`
	Ref    string    `json:"ref"`
}

// Validate applies the entry rules. The sequence is not one of them: Append assigns it.
func (e Entry) Validate() error {
	switch {
	case e.At.IsZero():
		return fmt.Errorf("%w: no instant", ErrInvalid)
	case e.Action == "" || len(e.Action) > 64:
		return fmt.Errorf("%w: action %q", ErrInvalid, e.Action)
	case e.Tenant == "" || len(e.Tenant) > 64:
		return fmt.Errorf("%w: tenant %q", ErrInvalid, e.Tenant)
	case e.Actor == "" || len(e.Actor) > 255:
		return fmt.Errorf("%w: actor %q", ErrInvalid, e.Actor)
	case !refRE.MatchString(e.Ref):
		return fmt.Errorf("%w: ref %q", ErrInvalid, e.Ref)
	}
	return nil
}

// Journal is the trail. It is safe for concurrent use.
type Journal struct {
	mu      sync.RWMutex
	entries []Entry
	next    int64
}

// New returns a journal whose first entry carries sequence first.
func New(first int64) *Journal {
	if first <= 0 {
		first = 1
	}
	return &Journal{entries: make([]Entry, 0, 64), next: first}
}

// Append records one entry and returns it with the sequence assigned. The sequence is checked at the top of
// the range: a wrapped sequence would make two entries indistinguishable, which is worse than refusing the
// write.
func (j *Journal) Append(entry Entry) (Entry, error) {
	if err := entry.Validate(); err != nil {
		return Entry{}, err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.next == math.MaxInt64 {
		return Entry{}, ErrExhausted
	}
	if len(j.entries) >= MaxEntries {
		return Entry{}, ErrFull
	}
	entry.Seq = j.next
	j.next++
	j.entries = append(j.entries, entry)
	return entry, nil
}

// Len reports how many entries the trail carries.
func (j *Journal) Len() int {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return len(j.entries)
}
