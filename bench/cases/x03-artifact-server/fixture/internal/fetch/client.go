// Package fetch is the bounded HTTP client the artifact server calls an endpoint with. It keeps a
// delivery's body, timeout and response size bounded, and it takes the doer as a value so a test can
// observe the requests a delivery would make without a network.
package fetch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Doer performs one request. http.Client implements it, and so does a test's recorder.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client is the delivery client.
type Client struct {
	doer    Doer
	timeout time.Duration
	maxBody int64
}

// New returns a client over a doer.
func New(doer Doer) *Client {
	if doer == nil {
		doer = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{doer: doer, timeout: 10 * time.Second, maxBody: 1 << 20}
}

// SetTimeout replaces the ceiling on one request.
func (c *Client) SetTimeout(timeout time.Duration) {
	if timeout > 0 {
		c.timeout = timeout
	}
}

// SetMaxBody replaces the number of response bytes the client keeps.
func (c *Client) SetMaxBody(limit int64) {
	if limit > 0 {
		c.maxBody = limit
	}
}

// Response is what a delivery got back.
type Response struct {
	Status int
	Host   string
	Body   string
}

// Post sends one JSON body to an endpoint and returns the answer. The endpoint is the URL the caller
// already decided may be called; this client does not decide anything about where a request goes.
func (c *Client) Post(ctx context.Context, endpoint string, body []byte, headers map[string]string) (Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("fetch: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "artifactd/1")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := c.doer.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("fetch: post %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	answer := Response{Status: resp.StatusCode, Host: req.URL.Hostname()}
	if c.maxBody > 0 {
		body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody))
		if err != nil {
			return answer, fmt.Errorf("fetch: read %s: %w", endpoint, err)
		}
		answer.Body = strings.TrimSpace(string(body))
	}
	return answer, nil
}

// Accepted reports whether a status is one an endpoint answers a delivery with.
func Accepted(status int) bool { return status >= 200 && status < 300 }

// Recorded is a doer that answers every request with the status given and remembers what it was
// asked. A deployment never uses it; the trigger harness and the tests do, so a delivery to an address
// that must not be called is observed instead of attempted.
type Recorded struct {
	Status    int
	Requests  []string
	Bodies    []string
	Responses int
}

// NewRecorded returns a recorder that answers with status.
func NewRecorded(status int) *Recorded {
	return &Recorded{Status: status, Requests: make([]string, 0, 4), Bodies: make([]string, 0, 4)}
}

// Do records the request and answers it without a network.
func (r *Recorded) Do(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	r.Requests = append(r.Requests, req.Method+" "+req.URL.String())
	r.Bodies = append(r.Bodies, string(body))
	r.Responses++
	header := make(http.Header)
	header.Set("Content-Type", "application/json")
	return &http.Response{
		StatusCode: r.Status,
		Status:     fmt.Sprintf("%d %s", r.Status, http.StatusText(r.Status)),
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		Header:     header,
		Request:    req,
	}, nil
}

// Last returns the last request recorded, or the empty string.
func (r *Recorded) Last() string {
	if len(r.Requests) == 0 {
		return ""
	}
	return r.Requests[len(r.Requests)-1]
}
