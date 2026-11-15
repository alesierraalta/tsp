package auth

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// The scopes the billing gateway grants. A scope is a set of exact strings: "billing:readonly" grants
// nothing that "billing:read" requires.
const (
	// ScopeBillingRead reads plans, accounts and subscriptions.
	ScopeBillingRead = "billing:read"
	// ScopeBillingWrite changes subscriptions, including their seats.
	ScopeBillingWrite = "billing:write"
	// ScopeInvoicesRead reads invoices and their payments.
	ScopeInvoicesRead = "invoices:read"
	// ScopeInvoicesWrite records payments and issues invoices.
	ScopeInvoicesWrite = "invoices:write"
	// ScopeReportsRead reads the reports.
	ScopeReportsRead = "reports:read"
	// ScopeReportsExport downloads a report as CSV.
	ScopeReportsExport = "reports:export"
	// ScopeUsageWrite records usage events.
	ScopeUsageWrite = "usage:write"
	// ScopeAuditRead reads the trail.
	ScopeAuditRead = "audit:read"
	// ScopeAdminOperate runs the operator routes.
	ScopeAdminOperate = "admin:operate"
)

// scopeRe is the shape a scope has: a resource and an action, lower-case, separated by a colon.
var scopeRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}:[a-z][a-z0-9_-]{0,31}$`)

// ValidScope reports whether value is a scope this service would grant.
func ValidScope(value string) bool {
	if len(value) > 64 {
		return false
	}
	return scopeRe.MatchString(value)
}

// Set is a set of scopes.
type Set map[string]bool

// ParseScopes builds a set and refuses a scope that is not one this service grants. A scope that was
// silently dropped would make a token carry fewer rights than its issuer wrote, and a scope that was
// accepted without validation would let a caller grant itself an action the policy never named.
func ParseScopes(values []string) (Set, error) {
	set := make(Set, len(values))
	for _, value := range values {
		scope := strings.TrimSpace(value)
		if scope == "" {
			continue
		}
		if !ValidScope(scope) {
			return nil, fmt.Errorf("auth: %q is not a scope", value)
		}
		set[scope] = true
	}
	return set, nil
}

// MustParseScopes is ParseScopes for a deployment's own table, where a bad scope is a programming
// error rather than a request.
func MustParseScopes(values ...string) Set {
	set, err := ParseScopes(values)
	if err != nil {
		panic(err)
	}
	return set
}

// Has reports whether the set carries a scope.
func (s Set) Has(scope string) bool { return s[scope] }

// Requires reports whether the set carries every scope named.
func (s Set) Requires(scopes ...string) bool {
	for _, scope := range scopes {
		if !s.Has(scope) {
			return false
		}
	}
	return true
}

// Add returns a set carrying one more scope.
func (s Set) Add(scope string) Set {
	out := s.Clone()
	out[scope] = true
	return out
}

// Clone returns a copy of the set.
func (s Set) Clone() Set {
	out := make(Set, len(s))
	for scope := range s {
		out[scope] = true
	}
	return out
}

// Names returns the scopes in order, which is what a token's claims carry and what a console prints.
func (s Set) Names() []string {
	names := make([]string, 0, len(s))
	for scope := range s {
		names = append(names, scope)
	}
	sort.Strings(names)
	return names
}

// Len returns the number of scopes in the set.
func (s Set) Len() int { return len(s) }

// Empty reports whether the set carries nothing.
func (s Set) Empty() bool { return len(s) == 0 }
