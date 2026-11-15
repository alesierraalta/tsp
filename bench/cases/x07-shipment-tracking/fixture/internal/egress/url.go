// Package egress is the host boundary: the only code that decides whether an outbound endpoint may be
// contacted. The decision is made on a parsed URL, never on the string the caller wrote, because a
// string that looks like one host can parse into another — a userinfo section, an escaped host, a
// trailing dot. The host rule is a dotted suffix comparison, so a partner listed as `example.com` allows
// `hooks.example.com` and never `notexample.com`; an address literal is only ever allowed exactly.
package egress

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Errors the guard reports.
var (
	ErrURL     = errors.New("egress: endpoint refused")
	ErrHost    = errors.New("egress: host is not a partner")
	ErrScheme  = errors.New("egress: scheme refused")
	ErrShape   = errors.New("egress: endpoint shape refused")
	ErrNoGuard = errors.New("egress: no partner list")
)

// Guard decides which endpoints a deployment may contact. The partner list is fixed at open time; a
// request names an endpoint and the guard says whether it is one of the partners' hosts.
type Guard struct {
	partners  []string
	allowHTTP bool
}

// NewGuard builds a guard from the partner hosts a deployment configured. Each entry is a bare host
// name: a value carrying a scheme, a path, a port, a userinfo or a leading dot is a configuration
// mistake and is refused, because an entry that is not a host cannot be compared with one.
func NewGuard(partners []string, allowHTTP bool) (*Guard, error) {
	if len(partners) == 0 {
		return nil, ErrNoGuard
	}
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(partners))
	for _, entry := range partners {
		host := strings.TrimSpace(entry)
		if host == "" {
			continue
		}
		if host != strings.ToLower(host) {
			return nil, fmt.Errorf("%w: %q is not lowercase", ErrHost, entry)
		}
		if strings.ContainsAny(host, ":/@?#\\") || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
			return nil, fmt.Errorf("%w: %q is not a bare host", ErrHost, entry)
		}
		if strings.ContainsAny(host, " \t") || len(host) > 253 {
			return nil, fmt.Errorf("%w: %q is not a host", ErrHost, entry)
		}
		if !seen[host] {
			seen[host] = true
			cleaned = append(cleaned, host)
		}
	}
	if len(cleaned) == 0 {
		return nil, ErrNoGuard
	}
	return &Guard{partners: cleaned, allowHTTP: allowHTTP}, nil
}

// Partners lists the configured hosts.
func (g *Guard) Partners() []string { return append([]string(nil), g.partners...) }

// Insecure reports whether the deployment allows a plain-http partner.
func (g *Guard) Insecure() bool { return g.allowHTTP }

// Parse reads an endpoint string and decides whether it may be contacted. Parsing before the decision is
// the point: the guard reads the scheme, the host, the port and the path the URL actually has, not the
// pieces of the string the caller meant.
func (g *Guard) Parse(endpoint string) (*url.URL, error) {
	if endpoint == "" || len(endpoint) > 2048 {
		return nil, fmt.Errorf("%w: %d bytes", ErrShape, len(endpoint))
	}
	if strings.TrimSpace(endpoint) != endpoint {
		return nil, fmt.Errorf("%w: endpoint carries surrounding space", ErrShape)
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrShape, err)
	}
	if err := g.Check(parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

// Check decides whether a parsed URL may be contacted. Every rule reads a parsed component:
//
//   - the scheme is https, or http only when the deployment allows an insecure partner;
//   - there is no userinfo, no fragment and no opaque part, so the URL cannot be read as one host by a
//     redirect and contacted as another by the client;
//   - the path is absolute and carries no control character or backslash;
//   - the host is one of the partners, compared as a dotted suffix: `example.com` allows
//     `hooks.example.com` and refuses `notexample.com`, and an address literal must match exactly;
//   - the port is 443 for https. A plain-http endpoint is only reached by an insecure deployment, and
//     only a host the list carries exactly may use a port other than 80: a private partner on another
//     port is a named host, while a suffix match is allowed the default port only.
func (g *Guard) Check(parsed *url.URL) error {
	if parsed == nil {
		return fmt.Errorf("%w: no URL", ErrShape)
	}
	switch parsed.Scheme {
	case "https":
	case "http":
		if !g.allowHTTP {
			return fmt.Errorf("%w: http needs an insecure deployment", ErrScheme)
		}
	default:
		return fmt.Errorf("%w: %q", ErrScheme, parsed.Scheme)
	}
	if parsed.User != nil {
		return fmt.Errorf("%w: userinfo", ErrShape)
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("%w: fragment", ErrShape)
	}
	if parsed.Opaque != "" {
		return fmt.Errorf("%w: opaque URL", ErrShape)
	}
	if parsed.Path == "" || !strings.HasPrefix(parsed.Path, "/") {
		return fmt.Errorf("%w: path %q is not absolute", ErrShape, parsed.Path)
	}
	if parsed.RawPath != "" {
		return fmt.Errorf("%w: escaped path", ErrShape)
	}
	for _, r := range parsed.Path {
		if r < 0x20 || r == 0x7f || r == '\\' {
			return fmt.Errorf("%w: path carries a control character", ErrShape)
		}
	}
	for _, r := range parsed.RawQuery {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%w: query carries a control character", ErrShape)
		}
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("%w: no host", ErrShape)
	}
	if strings.HasSuffix(host, ".") {
		// A fully qualified name with a trailing dot names the same host to a resolver and a different
		// string to a comparison, so it is refused instead of being trimmed into a match.
		return fmt.Errorf("%w: %q ends with a dot", ErrHost, host)
	}
	exact, err := g.hostAllowed(host)
	if err != nil {
		return err
	}
	port := parsed.Port()
	if parsed.Scheme == "https" {
		if port != "" && port != "443" {
			return fmt.Errorf("%w: port %q on https", ErrShape, port)
		}
		return nil
	}
	if port != "" && port != "80" && !exact {
		return fmt.Errorf("%w: port %q needs a host the list carries exactly", ErrShape, port)
	}
	return nil
}

// hostAllowed compares a parsed host with the partner list. An address literal is compared exactly and
// never by suffix: `1.2.3.4` ends with `.2.3.4` as a string, so a list entry of `2.3.4` must not reach an
// address it does not name. A name is allowed when the list carries it exactly or carries a parent domain
// the name sits under — the comparison includes the dot, which is the whole rule.
func (g *Guard) hostAllowed(host string) (exact bool, err error) {
	lowered := strings.ToLower(host)
	isAddress := net.ParseIP(host) != nil
	for _, partner := range g.partners {
		if partner == lowered {
			return true, nil
		}
		if isAddress {
			// An entry that is not this address is not a parent of it: an address has no children.
			continue
		}
		if strings.HasSuffix(lowered, "."+partner) {
			return false, nil
		}
	}
	return false, fmt.Errorf("%w: %q", ErrHost, host)
}
