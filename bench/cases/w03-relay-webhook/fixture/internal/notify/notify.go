// Package notify delivers the events of the service to the endpoints a subscription names.
package notify

import (
	"encoding/json"
	"fmt"
	"time"
)

// Event is one thing that happened, ready to be delivered.
type Event struct {
	ID      string    `json:"id"`
	Kind    string    `json:"kind"`
	Tenant  string    `json:"tenant"`
	Payload []byte    `json:"payload"`
	At      time.Time `json:"at"`
}

// Subscription is one endpoint a tenant wants its events delivered to.
type Subscription struct {
	ID       string   `json:"id"`
	Tenant   string   `json:"tenant"`
	Endpoint string   `json:"endpoint"`
	Secret   string   `json:"-"`
	Kinds    []string `json:"kinds"`
	Active   bool     `json:"active"`
}

// Wants reports whether the subscription asks for one event kind. A subscription with no kinds
// asks for none: an empty list is not "everything", because a subscription that quietly widened to
// every kind would deliver kinds its owner never asked for.
func (s Subscription) Wants(kind string) bool {
	for _, want := range s.Kinds {
		if want == kind {
			return true
		}
	}
	return false
}

// Validate refuses a subscription the service cannot deliver. The endpoint is not checked here:
// the outbound decision belongs to netguard, which is the one place that decides what may be
// called.
func (s Subscription) Validate() error {
	if s.ID == "" || s.Tenant == "" {
		return fmt.Errorf("%w: a subscription needs an id and a tenant", ErrInvalid)
	}
	if s.Endpoint == "" {
		return fmt.Errorf("%w: subscription %q has no endpoint", ErrInvalid, s.ID)
	}
	if len(s.Secret) < 16 {
		return fmt.Errorf("%w: subscription %q has a secret shorter than 16 bytes", ErrInvalid, s.ID)
	}
	return nil
}

// Delivery is the outcome of one delivery attempt sequence.
type Delivery struct {
	Subscription string
	Event        string
	Host         string
	Status       int
	Attempts     int
	Delivered    bool
}

// Envelope is the body the service posts: the event plus the subscription it belongs to.
type Envelope struct {
	Subscription string `json:"subscription"`
	Event        string `json:"event"`
	Kind         string `json:"kind"`
	Tenant       string `json:"tenant"`
	At           string `json:"at"`
	Payload      []byte `json:"payload"`
}

// Body renders the JSON body of one delivery.
func Body(sub Subscription, ev Event) ([]byte, error) {
	body, err := json.Marshal(Envelope{
		Subscription: sub.ID,
		Event:        ev.ID,
		Kind:         ev.Kind,
		Tenant:       ev.Tenant,
		At:           ev.At.UTC().Format(time.RFC3339),
		Payload:      ev.Payload,
	})
	if err != nil {
		return nil, fmt.Errorf("notify: encode event %q: %w", ev.ID, err)
	}
	return body, nil
}
