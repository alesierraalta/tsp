package notify

import (
	"context"
	"errors"
	"fmt"
)

// Fanout is what one broadcast came to: the deliveries and the endpoints that were refused.
type Fanout struct {
	Event      string
	Deliveries []Delivery
	Refused    []Refused
	Delivered  int
	Skipped    int
}

// Refused is one endpoint a broadcast did not reach.
type Refused struct {
	Subscription string
	Endpoint     string
	Reason       string
}

// Broadcast delivers one event to every active subscription of a project that asks for it.
//
// A broadcast is the fan-out path: an operator publishes one event and every subscriber hears it once.
// Each delivery goes through the same guarded path a re-delivery does, so the endpoint of every
// subscription is parsed and checked before anything is signed or sent, and one refused endpoint does
// not stop the others.
func (r *Relay) Broadcast(ctx context.Context, subs []Subscription, event Event) (Fanout, error) {
	if err := event.Validate(); err != nil {
		return Fanout{}, err
	}
	fanout := Fanout{Event: event.ID}
	var failures []error
	for _, sub := range subs {
		if sub.Project != event.Project || !sub.Wants(event.Kind) {
			fanout.Skipped++
			continue
		}
		delivery, err := r.Deliver(ctx, sub, event)
		fanout.Deliveries = append(fanout.Deliveries, delivery)
		if err != nil {
			fanout.Refused = append(fanout.Refused, Refused{
				Subscription: sub.ID, Endpoint: sub.Endpoint, Reason: err.Error(),
			})
			failures = append(failures, fmt.Errorf("%s: %w", sub.ID, err))
			continue
		}
		fanout.Delivered++
	}
	if len(fanout.Deliveries) == 0 {
		return fanout, fmt.Errorf("%w: no subscription of project %q asks for %q",
			ErrNotWanted, event.Project, event.Kind)
	}
	if len(failures) > 0 {
		return fanout, fmt.Errorf("notify: %w", errors.Join(failures...))
	}
	return fanout, nil
}

// DeliverAll delivers one event to the subscriptions given, through the guarded delivery path, and
// reports one line per subscription. The operator route that re-delivers an event uses it.
func (r *Relay) DeliverAll(ctx context.Context, subs []Subscription, event Event) (Fanout, error) {
	fanout := Fanout{Event: event.ID}
	var failures []error
	for _, sub := range subs {
		if !Matches(sub, event) {
			fanout.Skipped++
			continue
		}
		delivery, err := r.Deliver(ctx, sub, event)
		fanout.Deliveries = append(fanout.Deliveries, delivery)
		if err != nil {
			fanout.Refused = append(fanout.Refused, Refused{
				Subscription: sub.ID, Endpoint: sub.Endpoint, Reason: err.Error(),
			})
			failures = append(failures, fmt.Errorf("%s: %w", sub.ID, err))
			continue
		}
		fanout.Delivered++
	}
	if len(fanout.Deliveries) == 0 {
		return fanout, fmt.Errorf("%w: no subscription asks for %q", ErrNotWanted, event.Kind)
	}
	if len(failures) > 0 {
		return fanout, fmt.Errorf("notify: %w", errors.Join(failures...))
	}
	return fanout, nil
}
