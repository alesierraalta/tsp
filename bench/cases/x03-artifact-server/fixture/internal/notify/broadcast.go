package notify

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"example.com/artifacts/internal/fetch"
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
// The endpoint of each subscription is checked for a secure scheme before the body is rendered, and
// the delivery is reported per subscription so one refused endpoint does not stop the others.
func (r *Relay) Broadcast(ctx context.Context, subs []Subscription, event Event) (Fanout, error) {
	if err := event.Validate(); err != nil {
		return Fanout{}, err
	}
	fanout := Fanout{Event: event.ID}
	for _, sub := range subs {
		if sub.Project != event.Project || !sub.Wants(event.Kind) {
			fanout.Skipped++
			continue
		}
		if !sub.Active {
			fanout.Refused = append(fanout.Refused, Refused{
				Subscription: sub.ID, Endpoint: sub.Endpoint, Reason: "the subscription is inactive",
			})
			continue
		}
		delivery, err := r.spread(ctx, sub, event)
		if err != nil {
			fanout.Refused = append(fanout.Refused, Refused{
				Subscription: sub.ID, Endpoint: sub.Endpoint, Reason: err.Error(),
			})
			fanout.Deliveries = append(fanout.Deliveries, delivery)
			continue
		}
		fanout.Deliveries = append(fanout.Deliveries, delivery)
		if delivery.Delivered {
			fanout.Delivered++
		}
	}
	if len(fanout.Deliveries) == 0 {
		return fanout, fmt.Errorf("%w: no subscription of project %q asks for %q",
			ErrNotWanted, event.Project, event.Kind)
	}
	return fanout, nil
}

// spread posts one event to one subscription's stored endpoint.
func (r *Relay) spread(ctx context.Context, sub Subscription, event Event) (Delivery, error) {
	delivery := Delivery{Subscription: sub.ID, Event: event.ID}
	if !strings.HasPrefix(sub.Endpoint, "https://") {
		delivery.Err = "the endpoint is not a secure URL"
		return delivery, fmt.Errorf("%w: %q", ErrBlocked, sub.Endpoint)
	}
	body, err := Body(sub, event)
	if err != nil {
		delivery.Err = err.Error()
		return delivery, err
	}
	headers := map[string]string{
		"X-Artifact-Event":     event.Kind,
		"X-Artifact-Signature": Sign(sub.Secret, body),
	}
	answer, err := r.client.Post(ctx, sub.Endpoint, body, headers)
	delivery.Attempts = 1
	delivery.Status = answer.Status
	delivery.Host = answer.Host
	if err != nil {
		delivery.Err = err.Error()
		return delivery, err
	}
	if !fetch.Accepted(answer.Status) {
		delivery.Err = fmt.Sprintf("status %d from %s", answer.Status, answer.Host)
		return delivery, fmt.Errorf("%w: %s", ErrFailed, delivery.Err)
	}
	delivery.Delivered = true
	return delivery, nil
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
