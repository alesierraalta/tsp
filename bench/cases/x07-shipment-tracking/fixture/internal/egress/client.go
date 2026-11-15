package egress

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Errors the client reports.
var (
	ErrStatus   = errors.New("egress: endpoint answered with an error")
	ErrRedirect = errors.New("egress: redirect refused")
	ErrBody     = errors.New("egress: response is over the bound")
)

// Client posts to endpoints the guard allowed. Redirects are refused, so a partner that answers 302
// cannot send the request on to a host the deployment's list does not carry; the response is read
// through a bound one byte over the cap, so an oversized answer is reported rather than buffered.
type Client struct {
	guard       *Guard
	http        *http.Client
	maxResponse int64
}

// NewClient returns a client for a guard. The response cap must be inside 1 KiB..8 MiB, so a deployment
// cannot be configured to buffer a response a partner chooses the size of.
func NewClient(guard *Guard, maxResponse int64, timeout time.Duration) (*Client, error) {
	if guard == nil {
		return nil, ErrNoGuard
	}
	if maxResponse < 1024 || maxResponse > 8<<20 {
		return nil, fmt.Errorf("egress: response cap %d", maxResponse)
	}
	if timeout <= 0 || timeout > 2*time.Minute {
		return nil, fmt.Errorf("egress: timeout %s", timeout)
	}
	// The guard carries the deployment's scheme decision already: a client cannot widen it, only use it.
	return &Client{
		guard:       guard,
		maxResponse: maxResponse,
		http: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return ErrRedirect
			},
		},
	}, nil
}

// Response is what an endpoint answered: the status, the bytes and whether the body was cut.
type Response struct {
	Status    int
	Body      []byte
	Truncated bool
}

// Post sends a body to an endpoint the guard allowed. The endpoint is parsed and checked here, so a
// caller cannot hand the client a URL the guard never saw; the check happens again after any redirect
// the transport would follow, which is why redirects are refused instead.
func (c *Client) Post(ctx context.Context, endpoint string, body []byte) (Response, error) {
	if c.guard == nil {
		return Response{}, ErrNoGuard
	}
	target, err := c.guard.Parse(endpoint)
	if err != nil {
		return Response{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("%w: %v", ErrShape, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "shiptrack/1")
	answer, err := c.http.Do(request)
	if err != nil {
		if errors.Is(err, ErrRedirect) {
			return Response{}, ErrRedirect
		}
		return Response{}, fmt.Errorf("egress: post: %w", err)
	}
	defer answer.Body.Close()
	// One byte over the cap is read on purpose: a body of exactly the cap and a body above it are two
	// different answers, and a reader that stopped at the cap could not tell them apart.
	data, err := io.ReadAll(io.LimitReader(answer.Body, c.maxResponse+1))
	if err != nil {
		return Response{}, fmt.Errorf("egress: read response: %w", err)
	}
	truncated := int64(len(data)) > c.maxResponse
	if truncated {
		data = data[:c.maxResponse]
	}
	response := Response{Status: answer.StatusCode, Body: data, Truncated: truncated}
	if answer.StatusCode < 200 || answer.StatusCode > 299 {
		return response, fmt.Errorf("%w: status %d", ErrStatus, answer.StatusCode)
	}
	if truncated {
		return response, fmt.Errorf("%w: over %d bytes", ErrBody, c.maxResponse)
	}
	return response, nil
}

// Guard is the client's guard, for a caller that wants to check an endpoint before it builds a body.
func (c *Client) Guard() *Guard { return c.guard }

// MaxResponse is the cap the client applies.
func (c *Client) MaxResponse() int64 { return c.maxResponse }
