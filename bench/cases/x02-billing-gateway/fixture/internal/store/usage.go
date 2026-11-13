package store

import (
	"fmt"
	"sort"

	"example.com/billing/internal/clock"
)

// Event returns one usage event.
func (s *Store) Event(id string) (UsageEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	event, ok := s.events[id]
	if !ok {
		return UsageEvent{}, fmt.Errorf("%w: event %q", ErrNotFound, id)
	}
	return event, nil
}

// Events returns the events of a tenant in id order.
func (s *Store) Events(tenant string) []UsageEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]UsageEvent, 0, len(s.events))
	for _, event := range s.events {
		if event.Tenant == tenant {
			out = append(out, event)
		}
	}
	sortEvents(out)
	return out
}

// EventsIn returns the events of a tenant inside a period.
func (s *Store) EventsIn(tenant string, period clock.Period) []UsageEvent {
	all := s.Events(tenant)
	out := all[:0]
	for _, event := range all {
		if period.Contains(event.At) {
			out = append(out, event)
		}
	}
	return out
}

// EventsOf returns the events recorded against one subscription.
func (s *Store) EventsOf(tenant, subscription string) []UsageEvent {
	all := s.Events(tenant)
	out := all[:0]
	for _, event := range all {
		if event.Subscription == subscription {
			out = append(out, event)
		}
	}
	return out
}

// MarkBilled marks the events of a subscription inside a period as billed by the invoice given. An
// event is billed once: an event already carrying a status other than recorded is left alone, which
// is what keeps a second invoice for the same cycle from charging the same quantity twice.
func (s *Store) MarkBilled(tenant, subscription string, period clock.Period) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := 0
	for id, event := range s.events {
		if event.Tenant != tenant || event.Subscription != subscription {
			continue
		}
		if !period.Contains(event.At) || event.Status != EventRecorded {
			continue
		}
		event.Status = EventBilled
		s.events[id] = event
		changed++
	}
	if changed == 0 {
		return 0, nil
	}
	if err := s.refreshUsage(); err != nil {
		return 0, err
	}
	return changed, nil
}

// ArchiveBefore moves the events recorded before a day out of the current period. The events stay in
// the store for the retention window; only their status changes, so a report that asks for the
// recorded events of a cycle does not count an old one.
func (s *Store) ArchiveBefore(tenant string, day clock.Period) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := 0
	for id, event := range s.events {
		if event.Tenant != tenant || event.Status != EventRecorded {
			continue
		}
		if event.At.Before(day.Start) {
			event.Status = EventArchived
			s.events[id] = event
			changed++
		}
	}
	if changed == 0 {
		return 0, nil
	}
	if err := s.refreshUsage(); err != nil {
		return 0, err
	}
	return changed, nil
}

// UsageTotal returns the quantity one subscription recorded for a metric inside a period.
func (s *Store) UsageTotal(tenant, subscription, metric string, period clock.Period) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var total int64
	for _, event := range s.events {
		if event.Tenant != tenant || event.Subscription != subscription || event.Metric != metric {
			continue
		}
		if period.Contains(event.At) {
			total += event.Quantity
		}
	}
	return total
}

// sortEvents orders events by id.
func sortEvents(events []UsageEvent) {
	sort.Slice(events, func(i, j int) bool { return events[i].ID < events[j].ID })
}
