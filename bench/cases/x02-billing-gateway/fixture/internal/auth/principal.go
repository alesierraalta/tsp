package auth

import (
	"errors"
	"fmt"
)

// ErrDenied is the refusal of an action a principal may not perform.
var ErrDenied = errors.New("auth: the principal may not perform this action")

// Principal is the caller a request acts as: the subject, the tenant it may read and the scopes its
// token carries. The tenant of a request is the token's tenant and is never read from a body or a
// query parameter.
type Principal struct {
	Subject  string
	Tenant   string
	Scopes   Set
	Operator bool
}

// NewPrincipal builds a principal from verified claims. The operator flag is decided by the
// deployment's own list of operator subjects, not by a scope the token carries.
func NewPrincipal(claims Claims, operator bool) (Principal, error) {
	scopes, err := ParseScopes(claims.Scopes)
	if err != nil {
		return Principal{}, err
	}
	principal := Principal{
		Subject:  claims.Subject,
		Tenant:   claims.Tenant,
		Scopes:   scopes,
		Operator: operator,
	}
	if err := principal.Validate(); err != nil {
		return Principal{}, err
	}
	return principal, nil
}

// Validate checks the principal.
func (p Principal) Validate() error {
	if p.Subject == "" {
		return fmt.Errorf("%w: the principal has no subject", ErrDenied)
	}
	if p.Tenant == "" {
		return fmt.Errorf("%w: the principal has no tenant", ErrDenied)
	}
	return nil
}

// Anonymous returns the principal a request without a token acts as.
func Anonymous() Principal { return Principal{} }

// IsAnonymous reports whether the principal carries no subject.
func (p Principal) IsAnonymous() bool { return p.Subject == "" }

// May reports whether the principal may perform an action.
func (p Principal) May(policy *Policy, action Action) error {
	if p.IsAnonymous() {
		return fmt.Errorf("%w: the request carries no token", ErrDenied)
	}
	return policy.Allows(p, action)
}

// In reports whether the principal's tenant is the one given. A handler that resolved a record from a
// path calls this before it reads anything, so an id from another tenant is a not-found.
func (p Principal) In(tenant string) bool {
	if p.IsAnonymous() {
		return false
	}
	return p.Tenant == tenant
}

// ScopeNames lists the principal's scopes in order.
func (p Principal) ScopeNames() []string { return p.Scopes.Names() }
