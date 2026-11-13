package store

import (
	"fmt"
	"time"

	"example.com/shiptrack/internal/events"
	"example.com/shiptrack/internal/identity"
	"example.com/shiptrack/internal/money"
)

// AddEvent stores a scan. The scan's identity is what the carrier said happened, not the id the
// service minted, so a re-sent scan is refused and a partner's statement counts it once.
func (s *Store) AddEvent(e events.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.events[e.ID]; exists {
		return fmt.Errorf("%w: event %q", ErrDuplicate, e.ID)
	}
	key := events.Key(e)
	if _, exists := s.eventKeys[key]; exists {
		return fmt.Errorf("%w: %s at %s", ErrDuplicate, e.Kind, e.Instant.Format(time.RFC3339))
	}
	if len(s.events) >= s.maxEvents {
		return fmt.Errorf("%w: %d events", ErrFull, s.maxEvents)
	}
	s.events[e.ID] = e
	s.eventKeys[key] = e.ID
	s.eventsByShip[e.Shipment] = append(s.eventsByShip[e.Shipment], e)
	if newest, ok := s.eventTimes[e.Shipment]; !ok || e.Instant.After(newest) {
		s.eventTimes[e.Shipment] = e.Instant
	}
	if shipment, ok := s.shipments[e.Shipment]; ok && shipment.Status != string(e.Kind) {
		shipment.Status = events.Status(s.eventsByShip[e.Shipment])
		s.shipments[e.Shipment] = shipment
	}
	return nil
}

// Event resolves a scan by the identifier the service minted for it.
func (s *Store) Event(id string) (events.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	event, ok := s.events[id]
	if !ok {
		return events.Event{}, fmt.Errorf("%w: event %q", ErrNotFound, id)
	}
	return event, nil
}

// Events returns a shipment's scans in order.
func (s *Store) Events(shipment string) []events.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return events.Timeline(s.eventsByShip[shipment])
}

// EventCount reports how many scans a shipment carries.
func (s *Store) EventCount(shipment string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.eventsByShip[shipment])
}

// AddShipment stores a shipment the service accepted. A second shipment with the same id is a
// conflict, not a replacement: a shipment's created instant is what a period close bills against, and
// replacing it would move a billable shipment between periods.
func (s *Store) AddShipment(shipment Shipment) (Shipment, error) {
	if shipment.ID == "" || shipment.Partner == "" || shipment.Carrier == "" {
		return Shipment{}, fmt.Errorf("%w: shipment %q", ErrNotFound, shipment.ID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.shipments[shipment.ID]; ok {
		if existing.Tenant != shipment.Tenant || existing.Partner != shipment.Partner {
			return Shipment{}, fmt.Errorf("%w: shipment %q belongs to another partner", ErrConflict, shipment.ID)
		}
		return Shipment{}, fmt.Errorf("%w: shipment %q", ErrDuplicate, shipment.ID)
	}
	if len(s.shipments) >= s.maxShipments {
		return Shipment{}, fmt.Errorf("%w: %d shipments", ErrFull, s.maxShipments)
	}
	if shipment.Created.IsZero() {
		shipment.Created = time.Now().UTC()
	}
	if shipment.Status == "" {
		shipment.Status = events.Status(s.eventsByShip[shipment.ID])
	}
	s.shipments[shipment.ID] = shipment
	s.shipmentOrder = append(s.shipmentOrder, shipment.ID)
	return shipment, nil
}

// Shipment resolves a shipment by id. The caller compares its partner with the request's partner, so a
// token of one partner cannot read another partner's shipment by id.
func (s *Store) Shipment(id string) (Shipment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	shipment, ok := s.shipments[id]
	if !ok {
		return Shipment{}, fmt.Errorf("%w: shipment %q", ErrNotFound, id)
	}
	return shipment, nil
}

// Shipments lists a partner's shipments, newest first. An empty partner lists every shipment, which is
// what the operator console asks for.
func (s *Store) Shipments(tenant, partner string) []Shipment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]Shipment, 0, len(s.shipmentOrder))
	for _, id := range s.shipmentOrder {
		shipment := s.shipments[id]
		if tenant != "" && shipment.Tenant != tenant {
			continue
		}
		if partner != "" && shipment.Partner != partner {
			continue
		}
		list = append(list, shipment)
	}
	sortShipments(list)
	return list
}

// Status is the status of a shipment: its last scan's kind, or "in-transit" while it has none.
func (s *Store) Status(shipment string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return events.Status(s.eventsByShip[shipment])
}

// Due is the instant a shipment is due by: the instant of its latest scan, which is what a partner's
// timeline shows. It is not a promise; the service has no promised-delivery field.
func (s *Store) Due(shipment string) (time.Time, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	instant, ok := s.eventTimes[shipment]
	return instant, ok
}

// Price is the billed amount of a shipment. The store keeps no price: the tariff computes it at close,
// from the shipment's service and weight, so a rate change is one rule and not a migration.
func (s *Store) Price(partner identity.Partner, shipment Shipment, perKg money.Amount) (money.Amount, error) {
	kilograms := (shipment.Grams + 999) / 1000
	return money.MulInt(perKg, kilograms)
}
