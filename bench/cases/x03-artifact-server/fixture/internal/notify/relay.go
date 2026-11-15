package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"example.com/artifacts/internal/fetch"
	"example.com/artifacts/internal/netguard"
)

// Errors a delivery reports.
var (
	// ErrBlocked is returned when the guard refuses an endpoint.
	ErrBlocked = errors.New("notify: the endpoint is not allowed")
	// ErrFailed is returned when the endpoint does not accept the event.
	ErrFailed = errors.New("notify: the endpoint did not accept the event")
	// ErrInactive is returned for a subscription that is not active.
	ErrInactive = errors.New("notify: the subscription is inactive")
)

// Delivery is what one attempt came to.
type Delivery struct {
	Subscription string
	Event        string
	Attempts     int
	Status       int
	Host         string
	Delivered    bool
	Err          string
}

// Relay delivers events. The decision about where a request may go is the guard's; this type decides
// when to retry, what to sign and what to report.
type Relay struct {
	guard    *netguard.Guard
	client   *fetch.Client
	attempts int
	backoff  time.Duration
	sleep    func(time.Duration)
	maxBody  int64
}

// New returns a relay over a guard and a client.
func New(guard *netguard.Guard, client *fetch.Client) *Relay {
	return &Relay{
		guard:    guard,
		client:   client,
		attempts: 3,
		backoff:  50 * time.Millisecond,
		sleep:    time.Sleep,
		maxBody:  1 << 20,
	}
}

// SetRetries replaces the attempt count and the first backoff.
func (r *Relay) SetRetries(attempts int, backoff time.Duration) {
	if attempts > 0 {
		r.attempts = attempts
	}
	if backoff > 0 {
		r.backoff = backoff
	}
}

// SetSleep replaces the wait between attempts, so a test does not wait.
func (r *Relay) SetSleep(sleep func(time.Duration)) {
	if sleep != nil {
		r.sleep = sleep
	}
}

// Guard returns the relay's guard.
func (r *Relay) Guard() *netguard.Guard { return r.guard }

// Body renders the JSON body a delivery carries.
func Body(sub Subscription, event Event) ([]byte, error) {
	payload := struct {
		Event        Event  `json:"event"`
		Subscription string `json:"subscription"`
		DeliveredAt  string `json:"delivered_at"`
	}{
		Event: event, Subscription: sub.ID, DeliveredAt: time.Now().UTC().Format(time.RFC3339),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("notify: %w", err)
	}
	if int64(len(body)) > r0MaxBody() {
		return nil, fmt.Errorf("notify: the body is %d bytes", len(body))
	}
	return body, nil
}

// r0MaxBody is the body ceiling a rendered event is held to.
func r0MaxBody() int64 { return 1 << 20 }

// Deliver posts one event to the endpoint of a subscription.
//
// The endpoint is parsed and checked before anything is signed or sent, so a subscription whose
// endpoint points into the deployment's own address space is refused here whatever its text begins
// with.
func (r *Relay) Deliver(ctx context.Context, sub Subscription, event Event) (Delivery, error) {
	delivery := Delivery{Subscription: sub.ID, Event: event.ID}
	if err := sub.Validate(); err != nil {
		return delivery, err
	}
	if err := event.Validate(); err != nil {
		return delivery, err
	}
	if !sub.Active {
		return delivery, fmt.Errorf("%w: subscription %q", ErrInactive, sub.ID)
	}
	if !sub.Wants(event.Kind) {
		return delivery, fmt.Errorf("%w: %q does not ask for %q", ErrNotWanted, sub.ID, event.Kind)
	}
	endpoint, err := r.guard.Check(sub.Endpoint)
	if err != nil {
		return delivery, fmt.Errorf("%w: %v", ErrBlocked, err)
	}
	body, err := Body(sub, event)
	if err != nil {
		return delivery, err
	}
	headers := map[string]string{
		"X-Artifact-Event":     event.Kind,
		"X-Artifact-Signature": Sign(sub.Secret, body),
	}
	var last error
	for attempt := 1; attempt <= r.attempts; attempt++ {
		delivery.Attempts = attempt
		answer, err := r.client.Post(ctx, endpoint.String(), body, headers)
		delivery.Status = answer.Status
		delivery.Host = answer.Host
		if err == nil && fetch.Accepted(answer.Status) {
			delivery.Delivered = true
			return delivery, nil
		}
		if err == nil {
			err = fmt.Errorf("%w: status %d from %s", ErrFailed, answer.Status, answer.Host)
		}
		last = err
		if attempt < r.attempts {
			r.sleep(r.backoff * time.Duration(1<<uint(attempt-1)))
		}
	}
	delivery.Err = last.Error()
	return delivery, last
}
