// Package access decides who a request acts as and what that caller may read.
package access

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Errors the HTTP layer maps to status codes.
var (
	ErrUnauthenticated = errors.New("access: no verified caller")
	ErrUnknownSubject  = errors.New("access: unknown subject")
	ErrForbidden       = errors.New("access: scope not granted")
)

// Actions the reports need. A route names one and the policy decides the scopes it requires.
const (
	ActionRead     = "reports:read"
	ActionExport   = "reports:export"
	ActionQuota    = "quotas:read"
	ActionAccounts = "accounts:read"
)

// Scopes is the set of scopes a token carries. Membership is exact: a scope is granted by the
// string it is, never by a string it starts with.
type Scopes map[string]bool

// NewScopes builds a set from a scope list.
func NewScopes(list []string) Scopes {
	set := make(Scopes, len(list))
	for _, s := range list {
		if s = strings.TrimSpace(s); s != "" {
			set[s] = true
		}
	}
	return set
}

// Has reports whether the set carries one scope exactly.
func (s Scopes) Has(scope string) bool { return s[scope] }

// List returns the scopes in sorted order.
func (s Scopes) List() []string {
	out := make([]string, 0, len(s))
	for scope := range s {
		out = append(out, scope)
	}
	sort.Strings(out)
	return out
}

// Principal is a verified caller: the account, its tenant and its scopes.
type Principal struct {
	Subject string
	Tenant  string
	Role    string
	Scopes  Scopes
}

// Policy is the scope table of the service. It is the only place a route's scopes are decided.
type Policy struct {
	required map[string][]string
}

// NewPolicy returns the service policy.
func NewPolicy() *Policy {
	return &Policy{required: map[string][]string{
		ActionRead:     {"reports:read"},
		ActionExport:   {"reports:export", "reports:read"},
		ActionQuota:    {"quotas:read"},
		ActionAccounts: {"accounts:read"},
	}}
}

// Required returns the scopes an action needs, or nil for an action the policy does not know.
func (p *Policy) Required(action string) []string { return p.required[action] }

// Allows reports whether the principal holds every scope the action requires, exactly.
func (p *Policy) Allows(pr Principal, action string) bool {
	need, ok := p.required[action]
	if !ok || len(need) == 0 {
		return false
	}
	for _, scope := range need {
		if !pr.Scopes.Has(scope) {
			return false
		}
	}
	return true
}

// Check is Allows with an error a handler can return as it is.
func (p *Policy) Check(pr Principal, action string) error {
	if p.Allows(pr, action) {
		return nil
	}
	need := p.Required(action)
	if len(need) == 0 {
		return fmt.Errorf("%w: %q is not an action", ErrForbidden, action)
	}
	return fmt.Errorf("%w: %s needs %s, caller holds %s",
		ErrForbidden, action, strings.Join(need, "+"), strings.Join(pr.Scopes.List(), ","))
}
