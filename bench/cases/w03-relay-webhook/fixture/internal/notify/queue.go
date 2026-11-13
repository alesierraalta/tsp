package notify

import (
	"fmt"
	"sort"
	"sync"
)

// Registry is the subscription list of one tenant.
type Registry struct {
	mu       sync.RWMutex
	subs     map[string]Subscription
	byTenant map[string][]string
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{subs: map[string]Subscription{}, byTenant: map[string][]string{}}
}

// Add stores a subscription. A subscription that replaces an earlier one with the same id is
// refused: two subscriptions with one id would make a delivery's signature depend on which copy was
// read last.
func (r *Registry) Add(sub Subscription) error {
	if err := sub.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.subs[sub.ID]; ok {
		return fmt.Errorf("%w: subscription %q already exists", ErrInvalid, sub.ID)
	}
	r.subs[sub.ID] = sub
	r.byTenant[sub.Tenant] = append(r.byTenant[sub.Tenant], sub.ID)
	return nil
}

// Get returns one subscription.
func (r *Registry) Get(id string) (Subscription, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sub, ok := r.subs[id]
	return sub, ok
}

// OfTenant returns the subscriptions of one tenant, in id order.
func (r *Registry) OfTenant(tenant string) []Subscription {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Subscription, 0, len(r.byTenant[tenant]))
	for _, id := range r.byTenant[tenant] {
		out = append(out, r.subs[id])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Wanting returns the subscriptions of a tenant that ask for one event kind, in id order.
func (r *Registry) Wanting(tenant, kind string) []Subscription {
	var out []Subscription
	for _, sub := range r.OfTenant(tenant) {
		if sub.Active && sub.Wants(kind) {
			out = append(out, sub)
		}
	}
	return out
}

// Len reports how many subscriptions the registry carries.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.subs)
}

// Endpoints lists the endpoints the registry would call, sorted. The list is a value: a caller that
// wants to know what a deployment may reach reads the guard, not this.
func (r *Registry) Endpoints() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.subs))
	for _, sub := range r.subs {
		if sub.Active {
			out = append(out, sub.Endpoint)
		}
	}
	sort.Strings(out)
	return out
}
