// Package store is the service's in-memory record of what it has ingested, assigned, closed and
// delivered. It is the only place the records live, and it is read through the typed selection engine
// in select.go, so a report reads columns the table declares and never a string a request supplied.
package store

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"example.com/shiptrack/internal/clock"
	"example.com/shiptrack/internal/events"
	"example.com/shiptrack/internal/money"
)

// Errors the store reports.
var (
	ErrDuplicate = errors.New("store: already recorded")
	ErrNotFound  = errors.New("store: not found")
	ErrFull      = errors.New("store: capacity reached")
	ErrConflict  = errors.New("store: conflicting record")
)

// Shipment is one shipment the service has accepted. Its weight is grams, its lane is display text and
// its status is the last scan's kind, or "in-transit" while there is none.
type Shipment struct {
	ID      string
	Tenant  string
	Partner string
	Carrier string
	Lane    string
	Service string
	Grams   int64
	Created time.Time
	Status  string
}

// Statement is one closed statement: the totals the close computed and the objects it wrote.
type Statement struct {
	ID      string
	Tenant  string
	Partner string
	Period  clock.Period
	Lines   int
	Net     money.Amount
	Tax     money.Amount
	Total   money.Amount
	Written time.Time
	Paths   []string
}

// Delivery is one attempted partner notification.
type Delivery struct {
	ID       string
	Partner  string
	Endpoint string
	Status   string
	Attempts int
	At       time.Time
}

// Store holds the records. Reads take a read lock and writes take the write lock, so a handler and a
// close can run at the same time without either seeing a half-written record.
type Store struct {
	mu             sync.RWMutex
	events         map[string]events.Event
	eventKeys      map[string]string
	eventsByShip   map[string][]events.Event
	eventTimes     map[string]time.Time
	shipments      map[string]Shipment
	shipmentOrder  []string
	statements     map[string]Statement
	statementOrder []string
	deliveries     map[string]Delivery
	deliveryOrder  []string
	maxEvents      int
	maxShipments   int
}

// New returns an empty store. The caps are hard ceilings: at the ceiling the store refuses a new record
// rather than evicting one, because an evicted scan is a timeline that cannot be rebuilt.
func New(maxEvents, maxShipments int) (*Store, error) {
	if maxEvents < 1 || maxEvents > 10000000 {
		return nil, fmt.Errorf("store: event cap %d", maxEvents)
	}
	if maxShipments < 1 || maxShipments > 1000000 {
		return nil, fmt.Errorf("store: shipment cap %d", maxShipments)
	}
	return &Store{
		events:       map[string]events.Event{},
		eventKeys:    map[string]string{},
		eventsByShip: map[string][]events.Event{},
		eventTimes:   map[string]time.Time{},
		shipments:    map[string]Shipment{},
		statements:   map[string]Statement{},
		deliveries:   map[string]Delivery{},
		maxEvents:    maxEvents,
		maxShipments: maxShipments,
	}, nil
}

func statementKey(tenant, partner, period string) string {
	return tenant + "\x00" + partner + "\x00" + period
}

// Stats reports the record counts a console prints.
func (s *Store) Stats() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]int{
		"events":     len(s.events),
		"shipments":  len(s.shipments),
		"statements": len(s.statements),
		"deliveries": len(s.deliveries),
	}
}

// StatementCount reports how many statements the store holds.
func (s *Store) StatementCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.statements)
}

// DeliveryCount reports how many deliveries the store holds.
func (s *Store) DeliveryCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.deliveries)
}

// ShipmentIDs lists the shipments in creation order.
func (s *Store) ShipmentIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.shipmentOrder...)
}

// NearestInstant is the newest instant the store holds for a shipment, or the zero instant.
func (s *Store) NearestInstant(shipment string) (time.Time, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	newest, ok := s.eventTimes[shipment]
	return newest, ok
}

// sortShipments orders a list of shipments by creation instant, newest first, then by id.
func sortShipments(list []Shipment) {
	sort.SliceStable(list, func(i, j int) bool {
		if !list[i].Created.Equal(list[j].Created) {
			return list[i].Created.After(list[j].Created)
		}
		return list[i].ID < list[j].ID
	})
}
