package events

import (
	"fmt"
	"sort"
	"sync"
)

// Index is a shipment's timeline and the set of scans it already holds. It exists so a re-sent scan
// is refused once, at ingest, instead of appearing twice on a statement: the key is what the carrier
// said happened (shipment, kind, instant, facility), not the id the service minted for it, because a
// re-send mints nothing and carries no id.
type Index struct {
	mu       sync.RWMutex
	byKey    map[string]Event
	timeline map[string][]Event
	max      int
}

// NewIndex returns an empty index capped at max events. The cap is a hard ceiling: at the cap the
// index refuses a new event rather than dropping an old one, because a dropped scan is a timeline a
// partner cannot reconstruct.
func NewIndex(max int) (*Index, error) {
	if max < 1 || max > 10000000 {
		return nil, fmt.Errorf("%w: index cap %d", ErrInvalid, max)
	}
	return &Index{byKey: map[string]Event{}, timeline: map[string][]Event{}, max: max}, nil
}

// Key is the identity of a scan: two events with the same key are the same scan, re-sent.
func Key(e Event) string {
	return e.Shipment + "\x00" + string(e.Kind) + "\x00" + e.Instant.UTC().Format("2006-01-02T15:04:05.999999999Z07:00") + "\x00" + e.Facility
}

// Add stores an event, refusing a scan the index already holds.
func (x *Index) Add(e Event) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	key := Key(e)
	if _, exists := x.byKey[key]; exists {
		return fmt.Errorf("%w: %s at %s", ErrDuplicate, e.Kind, e.Instant.Format("2006-01-02T15:04:05Z07:00"))
	}
	if len(x.byKey) >= x.max {
		return fmt.Errorf("%w: index holds %d events", ErrInvalid, x.max)
	}
	x.byKey[key] = e
	x.timeline[e.Shipment] = append(x.timeline[e.Shipment], e)
	return nil
}

// Has reports whether the index holds a scan with the event's key.
func (x *Index) Has(e Event) bool {
	x.mu.RLock()
	defer x.mu.RUnlock()
	_, ok := x.byKey[Key(e)]
	return ok
}

// Timeline returns a shipment's scans in order.
func (x *Index) Timeline(shipment string) []Event {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return Timeline(x.timeline[shipment])
}

// Shipments lists the shipments the index holds, sorted.
func (x *Index) Shipments() []string {
	x.mu.RLock()
	defer x.mu.RUnlock()
	names := make([]string, 0, len(x.timeline))
	for name := range x.timeline {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Len reports how many scans the index holds.
func (x *Index) Len() int {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return len(x.byKey)
}

// Status is the status of a shipment in the index, which is "in-transit" for a shipment the index
// holds no scan for.
func (x *Index) Status(shipment string) string { return Status(x.Timeline(shipment)) }
