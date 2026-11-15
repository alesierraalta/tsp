// Package fetch is the outbound HTTP client of the service. It is the only place a request leaves
// the process, and it only ever sends a URL a netguard Guard produced.
package fetch

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"example.com/relay/internal/netguard"
)

// Doer is the one method the service needs from an HTTP client. A deployment uses Client; a test
// and the trigger harness use their own, which is why the outbound decision is observable.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client is the production client: it checks every endpoint through the guard and bounds the body
// it reads back.
type Client struct {
	guard   *netguard.Guard
	client  Doer
	maxBody int64
}

// New returns a client over a guard and an HTTP client.
func New(guard *netguard.Guard, doer Doer, maxBody int64) *Client {
	if doer == nil {
		doer = &http.Client{Timeout: 10 * time.Second}
	}
	if maxBody <= 0 {
		maxBody = 1 << 20
	}
	return &Client{guard: guard, client: doer, maxBody: maxBody}
}

// Post sends a JSON body to an endpoint. The endpoint is checked here, so no caller can post to a
// URL the guard refused, and the body returned is bounded.
func (c *Client) Post(endpoint string, body []byte, headers map[string]string) (int, []byte, error) {
	target, err := c.guard.Check(endpoint)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequest(http.MethodPost, target.URL, bytes.NewReader(body))
	if err != nil {
		return 0, nil, fmt.Errorf("fetch: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("fetch: post %s: %w", target.Host, err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("fetch: read %s: %w", target.Host, err)
	}
	return resp.StatusCode, payload, nil
}

// MaxBody is the largest response body the client reads.
func (c *Client) MaxBody() int64 { return c.maxBody }
