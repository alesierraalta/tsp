package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// validateAddr checks a listen address: a port, optionally with a host, separated by a colon.
func validateAddr(addr string) error {
	if addr == "" {
		return fmt.Errorf("config: BILLING_ADDR is empty")
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("config: BILLING_ADDR %q is not host:port: %w", addr, err)
	}
	if host != "" && net.ParseIP(host) == nil && !validHostname(host) {
		return fmt.Errorf("config: BILLING_ADDR host %q is not a name or an address", host)
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return fmt.Errorf("config: BILLING_ADDR port %q is not a port number", port)
	}
	return nil
}

// validHostname reports whether name is a dotted name of letters, digits and hyphens.
func validHostname(name string) bool {
	if len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			switch {
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
			default:
				return false
			}
		}
	}
	return true
}

// validateTenants checks the tenant list: at least one tenant, each one a plain identifier, and no
// two that are the same bytes. A repeated tenant would answer for one tenant twice.
func validateTenants(tenants []string) error {
	if len(tenants) == 0 {
		return fmt.Errorf("config: BILLING_TENANTS is empty")
	}
	seen := make(map[string]bool, len(tenants))
	for _, tenant := range tenants {
		if !validIdentifier(tenant) {
			return fmt.Errorf("config: BILLING_TENANTS carries %q, which is not an identifier", tenant)
		}
		if seen[tenant] {
			return fmt.Errorf("config: BILLING_TENANTS carries %q twice", tenant)
		}
		seen[tenant] = true
	}
	return nil
}

// validIdentifier reports whether value is a short lowercase name: letters, digits, a hyphen and an
// underscore, starting with a letter.
func validIdentifier(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	if c := value[0]; c < 'a' || c > 'z' {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// validateSecret checks the token secret: sixteen bytes at least, and not a value an operator left
// in place from an example.
func validateSecret(secret string) error {
	if len(secret) < 16 {
		return fmt.Errorf("config: BILLING_TOKEN_SECRET is %d bytes, want at least 16", len(secret))
	}
	for _, weak := range []string{"change-me", "changeme", "secret", "example"} {
		if strings.EqualFold(secret, weak) || strings.HasPrefix(strings.ToLower(secret), weak+"-") {
			return fmt.Errorf("config: BILLING_TOKEN_SECRET is the example value %q", weak)
		}
	}
	return nil
}

// validateCurrency checks the currency code: three ASCII letters, upper-cased by the overlay.
func validateCurrency(code string) error {
	if len(code) != 3 {
		return fmt.Errorf("config: BILLING_CURRENCY is %q, want three letters", code)
	}
	for i := 0; i < len(code); i++ {
		if code[i] < 'A' || code[i] > 'Z' {
			return fmt.Errorf("config: BILLING_CURRENCY is %q, want three upper-case letters", code)
		}
	}
	return nil
}
