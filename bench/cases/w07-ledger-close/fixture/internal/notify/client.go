// Package notify delivers a statement to a partner endpoint. It is the only place a request leaves
// the process. An endpoint is decided on the parsed URL, not on the string that was sent: a check
// that reads the raw text sees neither userinfo nor the host a redirect would reach.
package notify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Errors a delivery reports.
var (
	ErrBlocked    = errors.New("notify: endpoint is not allowed")
	ErrMalformed  = errors.New("notify: endpoint is not a URL")
	ErrResponse   = errors.New("notify: response is larger than the bound")
	ErrPayload    = errors.New("notify: payload is larger than the bound")
	ErrRedirected = errors.New("notify: the partner redirected the delivery")
)

// The bounds one delivery carries.
const (
	MaxEndpointBytes = 2048
	MaxPayloadBytes  = 1 << 20
)

// Target is an endpoint that passed every check.
type Target struct {
	URL  string
	Host string
}

// Guard is the deployment's outbound policy: the partner hosts it may call.
type Guard struct {
	hosts map[string]bool
}

// NewGuard returns a guard over the host list. An entry that carries a scheme, a path, a port or
// space is refused at construction: an entry that silently disappeared would make the deployment
// call fewer partners than its operator wrote down, and an entry taken as a bare suffix would widen
// the policy.
func NewGuard(hosts []string) (*Guard, error) {
	guard := &Guard{hosts: make(map[string]bool, len(hosts))}
	for _, raw := range hosts {
		host := normalizeHost(raw)
		if host == "" || strings.ContainsAny(host, "/:@*? ") {
			return nil, fmt.Errorf("%w: partner entry %q", ErrMalformed, raw)
		}
		guard.hosts[host] = true
	}
	return guard, nil
}

// Allow reports whether a host is one the deployment may call. A name is allowed when the list
// carries it exactly, or when the list carries a parent domain and the name is under it. The dot is
// part of that comparison: "hooks.example.com" is under "example.com" because it ends with
// ".example.com", while "notexample.com" does not. An address literal is only ever allowed exactly,
// because "1.2.3.4" ends with ".2.3.4" as a string and must not be treated as a name under it.
func (g *Guard) Allow(host string) bool {
	host = normalizeHost(host)
	if host == "" {
		return false
	}
	if net.ParseIP(host) != nil {
		return g.hosts[host]
	}
	if g.hosts[host] {
		return true
	}
	for allowed := range g.hosts {
		if net.ParseIP(allowed) == nil && strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

// Check decides whether the service may call an endpoint. secureOnly is the deployment's floor:
// with it set, a plain-http endpoint is refused. A deployment that calls a partner inside a private
// network turns it off, and the allowlist still decides which host is reachable.
func (g *Guard) Check(raw string, secureOnly bool) (Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > MaxEndpointBytes {
		return Target{}, fmt.Errorf("%w: %d bytes", ErrMalformed, len(raw))
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return Target{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "https":
	case "http":
		if secureOnly {
			return Target{}, fmt.Errorf("%w: scheme %q is not https", ErrBlocked, parsed.Scheme)
		}
	default:
		return Target{}, fmt.Errorf("%w: scheme %q", ErrBlocked, parsed.Scheme)
	}
	if parsed.User != nil {
		// "https://partner.example.com@evil.example" parses with host "evil.example" and reads as the
		// partner to a person; the endpoint is refused rather than argued with.
		return Target{}, fmt.Errorf("%w: userinfo in %q", ErrBlocked, raw)
	}
	if parsed.Fragment != "" || parsed.Opaque != "" || !strings.HasPrefix(parsed.Path, "/") {
		return Target{}, fmt.Errorf("%w: %q is not a plain endpoint", ErrBlocked, raw)
	}
	host := normalizeHost(parsed.Hostname())
	if host == "" {
		return Target{}, fmt.Errorf("%w: %q has no host", ErrBlocked, raw)
	}
	if !g.Allow(host) {
		return Target{}, fmt.Errorf("%w: host %q is not a partner", ErrBlocked, host)
	}
	if port := parsed.Port(); port != "" {
		if !allDigits(port) || (scheme == "https" && port != "443") {
			return Target{}, fmt.Errorf("%w: port %q", ErrBlocked, port)
		}
	}
	return Target{URL: parsed.String(), Host: host}, nil
}

// normalizeHost lowercases a host and drops one trailing dot: a DNS name is case-insensitive and a
// fully qualified "hooks.example.com." is the same name as "hooks.example.com".
func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// Doer is the one method the client needs from an HTTP client. A deployment uses the client
// NewClient builds; a test passes its own, which is how the outbound decision is observable.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Result is what one delivery did.
type Result struct {
	Status    int
	Delivered bool
	Body      []byte
	Host      string
}

// Client posts payloads to partner endpoints.
type Client struct {
	guard       *Guard
	doer        Doer
	maxResponse int64
	secureOnly  bool
}

// NewClient returns a client. The HTTP client it builds refuses redirects: a 302 to another host
// would leave the process without passing Check, and the allowlist would then describe a policy the
// service does not keep.
func NewClient(guard *Guard, doer Doer, maxResponse int64, secureOnly bool) (*Client, error) {
	if guard == nil {
		return nil, fmt.Errorf("%w: no partner hosts", ErrBlocked)
	}
	if maxResponse <= 0 {
		maxResponse = 64 << 10
	}
	if doer == nil {
		doer = &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return ErrRedirected
			},
		}
	}
	return &Client{guard: guard, doer: doer, maxResponse: maxResponse, secureOnly: secureOnly}, nil
}

// Deliver posts a JSON payload to an endpoint and returns what came back. The endpoint is checked
// here even when the caller checked it: this is the last code before the request leaves the
// process, and a second decision that disagrees with the first is a refusal. The response body is
// read through a reader bounded by one byte over the limit, so the client learns that the answer
// was too large instead of buffering it.
func (c *Client) Deliver(ctx context.Context, endpoint string, payload []byte) (Result, error) {
	if len(payload) == 0 || len(payload) > MaxPayloadBytes {
		return Result{}, fmt.Errorf("%w: %d bytes", ErrPayload, len(payload))
	}
	target, err := c.guard.Check(endpoint, c.secureOnly)
	if err != nil {
		return Result{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.URL, bytes.NewReader(payload))
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "ledgerclose/1")
	response, err := c.doer.Do(request)
	if err != nil {
		if errors.Is(err, ErrRedirected) {
			return Result{}, err
		}
		return Result{}, fmt.Errorf("notify: post %s: %w", target.Host, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, c.maxResponse+1))
	if err != nil {
		return Result{}, fmt.Errorf("notify: read %s: %w", target.Host, err)
	}
	if int64(len(body)) > c.maxResponse {
		return Result{Status: response.StatusCode, Host: target.Host}, ErrResponse
	}
	return Result{
		Status:    response.StatusCode,
		Delivered: response.StatusCode >= 200 && response.StatusCode < 300,
		Body:      body,
		Host:      target.Host,
	}, nil
}
