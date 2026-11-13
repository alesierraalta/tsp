package notify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"example.com/relay/internal/fetch"
	"example.com/relay/internal/netguard"
)

// Errors a delivery reports.
var (
	ErrInvalid   = errors.New("notify: invalid subscription")
	ErrBlocked   = errors.New("notify: endpoint is not allowed")
	ErrNotWanted = errors.New("notify: subscription does not ask for this event")
	ErrFailed    = errors.New("notify: the endpoint did not accept the event")
)

// Notifier delivers events. The outbound decision is the guard's; this type only decides when to
// retry and what to sign.
type Notifier struct {
	guard    *netguard.Guard
	doer     fetch.Doer
	attempts int
	backoff  time.Duration
	sleep    func(time.Duration)
	maxBody  int64
}

// New returns a notifier over a guard and an HTTP client.
func New(guard *netguard.Guard, doer fetch.Doer) *Notifier {
	return &Notifier{
		guard:    guard,
		doer:     doer,
		attempts: 3,
		backoff:  50 * time.Millisecond,
		sleep:    time.Sleep,
		maxBody:  1 << 20,
	}
}

// SetRetries replaces the attempt count and the first backoff.
func (n *Notifier) SetRetries(attempts int, backoff time.Duration) {
	if attempts > 0 {
		n.attempts = attempts
	}
	if backoff > 0 {
		n.backoff = backoff
	}
}

// SetSleep replaces the wait between attempts so a test does not wait.
func (n *Notifier) SetSleep(sleep func(time.Duration)) {
	if sleep != nil {
		n.sleep = sleep
	}
}

// Deliver posts one event to the endpoint of a subscription.
//
// The endpoint is checked before anything is signed or sent, and only a secure endpoint is called.
func (n *Notifier) Deliver(ctx context.Context, sub Subscription, ev Event) (Delivery, error) {
	delivery := Delivery{Subscription: sub.ID, Event: ev.ID}
	if err := sub.Validate(); err != nil {
		return delivery, err
	}
	if !sub.Active {
		return delivery, fmt.Errorf("%w: subscription %q is inactive", ErrInvalid, sub.ID)
	}
	if !sub.Wants(ev.Kind) {
		return delivery, fmt.Errorf("%w: %q does not ask for %q", ErrNotWanted, sub.ID, ev.Kind)
	}
	target, err := n.guard.Check(sub.Endpoint)
	if err != nil {
		return delivery, err
	}
	delivery.Host = target.Host

	body, err := Body(sub, ev)
	if err != nil {
		return delivery, err
	}
	headers := map[string]string{
		"X-Relay-Event":     ev.Kind,
		"X-Relay-Signature": Sign(sub.Secret, body),
	}

	var lastErr error
	for attempt := 1; attempt <= n.attempts; attempt++ {
		delivery.Attempts = attempt
		status, host, err := n.post(ctx, target.URL, body, headers)
		delivery.Status = status
		if host != "" {
			delivery.Host = host
		}
		if err == nil && status >= 200 && status < 300 {
			delivery.Delivered = true
			return delivery, nil
		}
		if err == nil {
			err = fmt.Errorf("%w: status %d from %s", ErrFailed, status, host)
		}
		lastErr = err
		if attempt < n.attempts {
			n.sleep(n.backoff * time.Duration(1<<uint(attempt-1)))
		}
	}
	return delivery, lastErr
}

// post sends one attempt and returns the status, the host that answered and the error.
func (n *Notifier) post(ctx context.Context, endpoint string, body []byte, headers map[string]string) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, "", fmt.Errorf("notify: build request: %w", err)
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := n.doer.Do(req)
	if err != nil {
		return 0, req.URL.Hostname(), fmt.Errorf("notify: post: %w", err)
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, n.maxBody)); err != nil {
		return resp.StatusCode, req.URL.Hostname(), fmt.Errorf("notify: read response: %w", err)
	}
	return resp.StatusCode, req.URL.Hostname(), nil
}
