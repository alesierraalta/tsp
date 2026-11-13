// Package netguard is the artifact server's outbound policy: an endpoint may be called only when its
// parsed URL is https, carries no credentials, names a host on the deployment's allowlist and is not a
// private, loopback, link-local or metadata address. The decision is made on the parsed URL, because
// the parsed URL is what a request is sent to.
package netguard

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Errors the guard reports.
var (
	// ErrBlocked is returned for an endpoint the deployment may not call.
	ErrBlocked = errors.New("netguard: endpoint is not allowed")
	// ErrParse is returned for a URL that does not parse.
	ErrParse = errors.New("netguard: the endpoint is not a URL")
)

// Guard decides whether an endpoint may be called.
type Guard struct {
	allow    *Allowlist
	resolver Resolver
	// Port is the one port a delivery may use.
	Port string
}

// Resolver turns a host into the addresses it carries. A deployment uses the system resolver; a test
// hands over the addresses it wants the guard to see, so a name that resolves into a private range is
// refused without a network.
type Resolver func(host string) ([]net.IP, error)

// New returns a guard over an allowlist and a resolver. A guard without an allowlist refuses every
// endpoint: a delivery nobody listed is a delivery nobody agreed to.
func New(allow *Allowlist, resolver Resolver) *Guard {
	if allow == nil {
		allow = NewAllowlist(nil)
	}
	if resolver == nil {
		resolver = LookupHost
	}
	return &Guard{allow: allow, resolver: resolver, Port: "443"}
}

// SetPort replaces the port a delivery may use.
func (g *Guard) SetPort(port string) {
	if port != "" {
		g.Port = port
	}
}

// Allowlist returns the guard's allowlist.
func (g *Guard) Allowlist() *Allowlist { return g.allow }

// Check returns the URL a delivery may call, or the reason it may not.
//
// Every refusal names the part of the URL that failed, so an operator reading a blocked delivery knows
// whether to add a host to the allowlist, fix an endpoint the sender typed or look at why a name
// resolves into the cluster's own address space.
func (g *Guard) Check(raw string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: the endpoint is empty", ErrParse)
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParse, err)
	}
	if parsed.Scheme != "https" {
		return nil, fmt.Errorf("%w: scheme %q in %q", ErrBlocked, parsed.Scheme, raw)
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("%w: userinfo in %q", ErrBlocked, raw)
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" {
		return nil, fmt.Errorf("%w: no host in %q", ErrBlocked, raw)
	}
	if parsed.Port() != "" && parsed.Port() != g.Port {
		return nil, fmt.Errorf("%w: port %q in %q", ErrBlocked, parsed.Port(), raw)
	}
	if ip := net.ParseIP(host); ip != nil {
		if !Public(ip) {
			return nil, fmt.Errorf("%w: %q is a private or metadata address", ErrBlocked, host)
		}
	} else {
		if !g.allow.Has(host) {
			return nil, fmt.Errorf("%w: host %q is not on the allowlist", ErrBlocked, host)
		}
		addresses, err := g.resolver(host)
		if err != nil {
			return nil, fmt.Errorf("%w: %q does not resolve: %v", ErrBlocked, host, err)
		}
		for _, address := range addresses {
			if !Public(address) {
				return nil, fmt.Errorf("%w: %q resolves to the private address %s", ErrBlocked, host, address)
			}
		}
	}
	parsed.Fragment = ""
	return parsed, nil
}

// Allows reports whether an endpoint may be called, without returning the parsed URL.
func (g *Guard) Allows(raw string) bool {
	_, err := g.Check(raw)
	return err == nil
}

// LookupHost is the system resolver, used when a deployment does not hand one over.
func LookupHost(host string) ([]net.IP, error) {
	addresses, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("netguard: %q carries no address", host)
	}
	return addresses, nil
}
