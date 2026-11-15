package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// HeaderToken is the header the middleware reads the bearer token from.
const HeaderToken = "Authorization"

// HeaderTenant is the header a tenant-scoped deployment may pin the tenant with. A token's tenant
// always wins: the header can only narrow a session, never widen it.
const HeaderTenant = "X-Docvault-Tenant"

// Session is the verified caller of one request.
type Session struct {
	Principal Principal
	Expires   time.Time
}

// Resolver turns a request into a session. It is the one place the service reads credentials, so
// a handler never parses a header itself.
type Resolver struct {
	issuer *Issuer
	policy *Policy
}

// NewResolver returns a resolver over an issuer and a policy.
func NewResolver(issuer *Issuer, policy *Policy) *Resolver {
	return &Resolver{issuer: issuer, policy: policy}
}

// Resolve verifies the bearer token of a request and returns the session it names.
func (r *Resolver) Resolve(req *http.Request) (Session, error) {
	raw, err := Bearer(req.Header.Get(HeaderToken))
	if err != nil {
		return Session{}, err
	}
	tok, err := r.issuer.Verify(raw)
	if err != nil {
		return Session{}, err
	}
	pr := Principal{Subject: tok.Subject, Tenant: tok.Tenant, Scopes: NewScopeSet(tok.Scopes)}
	if pinned := strings.TrimSpace(req.Header.Get(HeaderTenant)); pinned != "" && pinned != pr.Tenant {
		return Session{}, fmt.Errorf("%w: header tenant %q does not match token tenant %q",
			ErrForbidden, pinned, pr.Tenant)
	}
	return Session{Principal: pr, Expires: tok.Expires}, nil
}

// Anonymous reports whether the session carries no verified caller. A handler that reads a public
// object still asks: the tenant of an anonymous session is empty, which matches no document.
func (s Session) Anonymous() bool { return s.Principal.Subject == "" }

// StatusCode maps an auth error to the HTTP status the service answers with.
func StatusCode(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden
	default:
		return http.StatusUnauthorized
	}
}
