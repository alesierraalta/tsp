// Package netguard decides which outbound endpoints the service may call. Every delivery, import
// feed and retry passes through Check before a request is made, so the decision lives here and not
// in the code that happens to build the URL.
package netguard

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Errors a check reports.
var (
	ErrBlocked   = errors.New("netguard: endpoint is not allowed")
	ErrMalformed = errors.New("netguard: endpoint is not a URL")
)

// Target is an endpoint that passed every check: the URL to call and the host it resolves to.
type Target struct {
	URL  string
	Host string
}

// Guard is the outbound policy of one deployment: an allowlist of hosts, and the rule that a
// private, loopback or link-local address is never called.
type Guard struct {
	allow *Allowlist
}

// New returns a guard over the hosts given.
func New(hosts []string) *Guard {
	return &Guard{allow: NewAllowlist(hosts)}
}

// Allowlist is the set of hosts a deployment may call.
func (g *Guard) Allowlist() *Allowlist { return g.allow }

// Check parses an endpoint and decides whether the service may call it. The checks are on the
// parsed URL, never on the string that was sent: userinfo, a trailing dot, an IPv6 literal and a
// port all parse into a host, and a check that reads the raw text sees none of them.
func (g *Guard) Check(raw string) (Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Target{}, fmt.Errorf("%w: empty", ErrMalformed)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return Target{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return Target{}, fmt.Errorf("%w: scheme %q is not https", ErrBlocked, parsed.Scheme)
	}
	if parsed.User != nil {
		return Target{}, fmt.Errorf("%w: userinfo in %q", ErrBlocked, raw)
	}
	host := parsed.Hostname()
	if host == "" {
		return Target{}, fmt.Errorf("%w: %q has no host", ErrBlocked, raw)
	}
	if !g.allow.Has(host) {
		return Target{}, fmt.Errorf("%w: host %q is not on the allowlist", ErrBlocked, host)
	}
	if ip := net.ParseIP(host); ip != nil && !publicAddress(ip) {
		return Target{}, fmt.Errorf("%w: host %q is not a public address", ErrBlocked, host)
	}
	if parsed.Port() != "" && parsed.Port() != "443" {
		return Target{}, fmt.Errorf("%w: port %q is not 443", ErrBlocked, parsed.Port())
	}
	return Target{URL: parsed.String(), Host: host}, nil
}

// CheckURL is Check for a caller that only needs the error.
func (g *Guard) CheckURL(raw string) error {
	_, err := g.Check(raw)
	return err
}
