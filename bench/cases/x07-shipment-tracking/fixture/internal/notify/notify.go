// Package notify delivers a partner's statement notice to the partner's endpoint. The endpoint is the
// partner's, not the request's: the service looks the partner up, reads the endpoint the deployment
// registered for it, and hands that string to internal/egress, which decides on the parsed URL whether
// the host is allowed. A delivery is recorded whether it succeeded or failed, so an operator can retry
// what failed without guessing which request left the process.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"example.com/shiptrack/internal/egress"
	"example.com/shiptrack/internal/identity"
	"example.com/shiptrack/internal/idgen"
)

// Errors a delivery reports.
var (
	ErrPayload = errors.New("notify: invalid payload")
	ErrRefused = errors.New("notify: delivery refused")
)

// Payload is the notice a partner receives. Every amount is rendered from the statement's integer minor
// units, and the statement itself is not attached: a partner downloads it with its token.
type Payload struct {
	Event     string `json:"event"`
	Partner   string `json:"partner"`
	Tenant    string `json:"tenant"`
	Period    string `json:"period"`
	Lines     int    `json:"lines"`
	Net       string `json:"net"`
	Tax       string `json:"tax"`
	Total     string `json:"total"`
	Currency  string `json:"currency"`
	Statement string `json:"statement"`
	SentAt    string `json:"sent_at"`
}

// Record is one attempted delivery.
type Record struct {
	ID       string
	Partner  string
	Endpoint string
	Status   string
	Attempts int
	At       time.Time
	Detail   string
}

// Queue delivers notices through a client that checks the host boundary. It retries an answer the
// partner can be expected to recover from — a connection failure or a 5xx — and does not retry a 4xx:
// a refused request is refused again, and repeating it is how a partner sees the same notice twice.
type Queue struct {
	client      *egress.Client
	maxAttempts int
	now         func() time.Time
	sleep       func(time.Duration)
}

// New returns a queue. A delivery is attempted at most maxAttempts times, and the cap is a hard
// ceiling: a request cannot ask for more attempts than the deployment allows.
func New(client *egress.Client, maxAttempts int) (*Queue, error) {
	if client == nil {
		return nil, fmt.Errorf("%w: no client", ErrRefused)
	}
	if maxAttempts < 1 || maxAttempts > 5 {
		return nil, fmt.Errorf("%w: attempts %d", ErrRefused, maxAttempts)
	}
	return &Queue{client: client, maxAttempts: maxAttempts, now: time.Now, sleep: time.Sleep}, nil
}

// SetClock replaces the clock a record reads and the wait between attempts, so a test does not sleep.
func (q *Queue) SetClock(now func() time.Time, sleep func(time.Duration)) {
	if now != nil {
		q.now = now
	}
	if sleep != nil {
		q.sleep = sleep
	}
}

// Deliver posts a payload to a partner's endpoint. The endpoint is read from the partner record, and a
// partner without one is refused before anything is marshalled: the service does not accept an endpoint
// from the request that asks for a delivery.
func (q *Queue) Deliver(ctx context.Context, partner identity.Partner, endpoint string, payload Payload) (Record, error) {
	if endpoint == "" {
		return Record{}, fmt.Errorf("%w: partner %q has no endpoint", ErrRefused, partner.ID)
	}
	if payload.Partner != partner.ID || payload.Tenant != partner.Tenant {
		return Record{}, fmt.Errorf("%w: payload names another partner", ErrPayload)
	}
	if payload.Event == "" || payload.Period == "" || payload.Statement == "" {
		return Record{}, fmt.Errorf("%w: event, period and statement are required", ErrPayload)
	}
	if payload.Lines < 0 {
		return Record{}, fmt.Errorf("%w: lines %d", ErrPayload, payload.Lines)
	}
	sent := q.now().UTC()
	payload.SentAt = sent.Format(time.RFC3339)
	body, err := json.Marshal(payload)
	if err != nil {
		return Record{}, fmt.Errorf("%w: %v", ErrPayload, err)
	}
	id, err := idgen.New("dlv")
	if err != nil {
		return Record{}, err
	}
	record := Record{ID: id, Partner: partner.ID, Endpoint: endpoint, At: sent}
	var lastErr error
	for attempt := 1; attempt <= q.maxAttempts; attempt++ {
		record.Attempts = attempt
		response, err := q.client.Post(ctx, endpoint, body)
		if err == nil {
			record.Status = "delivered"
			record.Detail = fmt.Sprintf("status %d, %d bytes", response.Status, len(response.Body))
			return record, nil
		}
		lastErr = err
		record.Status = "failed"
		record.Detail = err.Error()
		if !retryable(err) {
			break
		}
		if attempt < q.maxAttempts {
			q.sleep(time.Duration(attempt) * 250 * time.Millisecond)
		}
	}
	return record, fmt.Errorf("%w: %v", ErrRefused, lastErr)
}

// retryable reports whether a delivery error is one a second attempt could fix. A status error carries
// the answer's code; a refused host, a refused shape and a redirect are decisions, not failures, and
// are not retried.
func retryable(err error) bool {
	switch {
	case errors.Is(err, egress.ErrHost), errors.Is(err, egress.ErrShape), errors.Is(err, egress.ErrScheme),
		errors.Is(err, egress.ErrRedirect), errors.Is(err, egress.ErrNoGuard):
		return false
	case errors.Is(err, egress.ErrStatus):
		return true
	}
	// A connection failure, a timeout and a truncated response are worth one more attempt.
	return true
}

// Describe renders a record for a console row.
func (r Record) Describe() string {
	return fmt.Sprintf("%s %s %s (%d attempts)", r.Partner, r.Endpoint, r.Status, r.Attempts)
}
