package store

import (
	"fmt"
	"sort"
	"time"

	"example.com/shiptrack/internal/clock"
)

// RecordStatement stores a closed statement, replacing the statement of the same partner and period.
// A re-close is allowed and replaces: the second run is the one the store and the trail carry, because
// a statement that could be appended twice would double a partner's bill.
func (s *Store) RecordStatement(statement Statement) (Statement, error) {
	if statement.ID == "" || statement.Tenant == "" || statement.Partner == "" {
		return Statement{}, fmt.Errorf("%w: statement for partner %q", ErrNotFound, statement.Partner)
	}
	if !statement.Period.Valid() {
		return Statement{}, fmt.Errorf("%w: period %q", ErrNotFound, statement.Period)
	}
	if statement.Total.Negative() || statement.Net.Negative() || statement.Tax.Negative() {
		return Statement{}, fmt.Errorf("%w: statement totals are negative", ErrConflict)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := statementKey(statement.Tenant, statement.Partner, statement.Period.String())
	if _, exists := s.statements[key]; !exists {
		s.statementOrder = append(s.statementOrder, key)
	}
	s.statements[key] = statement
	return statement, nil
}

// Statement resolves the statement of a partner and period.
func (s *Store) Statement(tenant, partner string, period clock.Period) (Statement, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	statement, ok := s.statements[statementKey(tenant, partner, period.String())]
	return statement, ok
}

// Statements lists the statements of a tenant, newest period first. An empty tenant lists every
// statement, which is what the operator view asks for.
func (s *Store) Statements(tenant string) []Statement {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]Statement, 0, len(s.statementOrder))
	for _, key := range s.statementOrder {
		statement, ok := s.statements[key]
		if !ok {
			continue
		}
		if tenant != "" && statement.Tenant != tenant {
			continue
		}
		list = append(list, statement)
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Period != list[j].Period {
			return list[j].Period.Before(list[i].Period)
		}
		return list[i].Partner < list[j].Partner
	})
	return list
}

// RecordDelivery stores an attempted notification. A delivery is recorded whether it succeeded or not:
// an attempt that failed is what an operator retries, and a delivery that left no record cannot be
// retried without guessing.
func (s *Store) RecordDelivery(delivery Delivery) (Delivery, error) {
	if delivery.ID == "" || delivery.Partner == "" || delivery.Endpoint == "" {
		return Delivery{}, fmt.Errorf("%w: delivery %q", ErrNotFound, delivery.ID)
	}
	if delivery.At.IsZero() {
		delivery.At = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.deliveries[delivery.ID]; exists {
		return Delivery{}, fmt.Errorf("%w: delivery %q", ErrDuplicate, delivery.ID)
	}
	s.deliveries[delivery.ID] = delivery
	s.deliveryOrder = append(s.deliveryOrder, delivery.ID)
	return delivery, nil
}

// Delivery resolves a delivery by id.
func (s *Store) Delivery(id string) (Delivery, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	delivery, ok := s.deliveries[id]
	if !ok {
		return Delivery{}, fmt.Errorf("%w: delivery %q", ErrNotFound, id)
	}
	return delivery, nil
}

// Deliveries lists a partner's deliveries, newest first. An empty partner lists every delivery.
func (s *Store) Deliveries(partner string) []Delivery {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]Delivery, 0, len(s.deliveryOrder))
	for _, id := range s.deliveryOrder {
		delivery := s.deliveries[id]
		if partner != "" && delivery.Partner != partner {
			continue
		}
		list = append(list, delivery)
	}
	sort.SliceStable(list, func(i, j int) bool {
		if !list[i].At.Equal(list[j].At) {
			return list[i].At.After(list[j].At)
		}
		return list[i].ID < list[j].ID
	})
	return list
}
