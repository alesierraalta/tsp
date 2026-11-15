package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"example.com/billing/internal/auth"
	"example.com/billing/internal/httpx"
)

// authenticate reads the bearer token of a request and resolves the principal it names. The token is
// verified before any of its fields is used, and the operator flag comes from the deployment's own
// subject list rather than from the token.
func (a *API) authenticate(r *http.Request) (auth.Principal, error) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return auth.Principal{}, httpx.Unauthorized("the request carries no token")
	}
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return auth.Principal{}, httpx.Unauthorized("the authorization header is not a bearer token")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return auth.Principal{}, httpx.Unauthorized("the bearer token is empty")
	}
	claims, err := a.issuer.Verify(token)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrExpired):
			return auth.Principal{}, httpx.Unauthorized("the token has expired")
		case errors.Is(err, auth.ErrSignature):
			return auth.Principal{}, httpx.Unauthorized("the token signature does not verify")
		}
		return auth.Principal{}, httpx.Unauthorized("the token is not one this service issued")
	}
	if !a.cfg.HasTenant(claims.Tenant) {
		return auth.Principal{}, httpx.Forbidden("the token names a tenant this deployment does not serve")
	}
	principal, err := auth.NewPrincipal(claims, a.cfg.IsAdmin(claims.Subject))
	if err != nil {
		return auth.Principal{}, httpx.Unauthorized(err.Error())
	}
	return principal, nil
}

// tenantOf returns the principal's tenant, which is the only tenant a handler may read.
func tenantOf(principal auth.Principal) (string, error) {
	if principal.Tenant == "" {
		return "", httpx.Unauthorized(errNoTenant.Error())
	}
	return principal.Tenant, nil
}
