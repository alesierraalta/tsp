package store

import (
	"fmt"
	"sort"
	"time"

	"example.com/billing/internal/clock"
)

// PutSubscription stores one subscription and refreshes the projection.
func (s *Store) PutSubscription(sub Subscription) error {
	if err := sub.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureTenant(sub.Tenant); err != nil {
		return err
	}
	account, ok := s.accounts[sub.Account]
	if !ok || account.Tenant != sub.Tenant {
		return fmt.Errorf("%w: account %q in tenant %q", ErrNotFound, sub.Account, sub.Tenant)
	}
	if plan, ok := s.plans[sub.Plan]; !ok || plan.Tenant != sub.Tenant {
		return fmt.Errorf("%w: plan %q in tenant %q", ErrNotFound, sub.Plan, sub.Tenant)
	}
	if _, exists := s.subscriptions[sub.ID]; exists {
		return fmt.Errorf("%w: subscription %q", ErrDuplicate, sub.ID)
	}
	s.subscriptions[sub.ID] = sub
	return s.refresh()
}

// UpdateSubscription replaces a subscription the store carries, refreshing the projection.
func (s *Store) UpdateSubscription(sub Subscription) error {
	if err := sub.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.subscriptions[sub.ID]; !exists {
		return fmt.Errorf("%w: subscription %q", ErrNotFound, sub.ID)
	}
	s.subscriptions[sub.ID] = sub
	return s.refresh()
}

// Subscription returns one subscription.
func (s *Store) Subscription(id string) (Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sub, ok := s.subscriptions[id]
	if !ok {
		return Subscription{}, fmt.Errorf("%w: subscription %q", ErrNotFound, id)
	}
	return sub, nil
}

// SubscriptionInTenant returns a subscription only when it belongs to the tenant given.
func (s *Store) SubscriptionInTenant(tenant, id string) (Subscription, error) {
	sub, err := s.Subscription(id)
	if err != nil {
		return Subscription{}, err
	}
	if sub.Tenant != tenant {
		return Subscription{}, fmt.Errorf("%w: subscription %q in tenant %q", ErrNotFound, id, tenant)
	}
	return sub, nil
}

// Subscriptions returns the subscriptions of a tenant in id order.
func (s *Store) Subscriptions(tenant string) []Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Subscription, 0, len(s.subscriptions))
	for _, sub := range s.subscriptions {
		if sub.Tenant == tenant {
			out = append(out, sub)
		}
	}
	sortSubscriptions(out)
	return out
}

// SubscriptionsOf returns the subscriptions of one account.
func (s *Store) SubscriptionsOf(tenant, account string) []Subscription {
	all := s.Subscriptions(tenant)
	out := all[:0]
	for _, sub := range all {
		if sub.Account == account {
			out = append(out, sub)
		}
	}
	return out
}

// ActiveSubscriptionOf returns the subscription an account is billed through, which is the one a
// usage event is recorded against when the reporter does not name one.
func (s *Store) ActiveSubscriptionOf(tenant, account string) (Subscription, error) {
	for _, sub := range s.SubscriptionsOf(tenant, account) {
		if sub.Status == StatusActive {
			return sub, nil
		}
	}
	return Subscription{}, fmt.Errorf("%w: no active subscription for account %q", ErrNotFound, account)
}

// BillingCycle returns the cycle of a subscription that contains the instant given.
func (s *Store) BillingCycle(sub Subscription, at time.Time) (clock.Period, error) {
	return sub.Cycle(at)
}

// sortSubscriptions orders subscriptions by id.
func sortSubscriptions(subs []Subscription) {
	sort.Slice(subs, func(i, j int) bool { return subs[i].ID < subs[j].ID })
}
