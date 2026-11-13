// Package api is the HTTP surface: the routes, the token check each one requires and the handlers that
// turn a request into a store read or write. A handler does not decide a partner's identity from the
// request: the token names the partner, the directory resolves it, and every shipment, statement and
// report a handler reads is scoped to that partner. The tenant is the token's tenant as well, and the
// deployment must serve it.
package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"time"

	"example.com/shiptrack/internal/audit"
	"example.com/shiptrack/internal/auth"
	"example.com/shiptrack/internal/clock"
	"example.com/shiptrack/internal/config"
	"example.com/shiptrack/internal/httpx"
	"example.com/shiptrack/internal/identity"
	"example.com/shiptrack/internal/labels"
	"example.com/shiptrack/internal/notify"
	"example.com/shiptrack/internal/store"
	"example.com/shiptrack/internal/tariff"
	"example.com/shiptrack/internal/workspace"
)

// Deps is what the surface is wired from. Every field is required except Renderer (a deployment without
// a label binary answers 404 on the label route) and Notify (a deployment without a partner endpoint list
// refuses deliveries by name).
type Deps struct {
	Config    *config.Config
	Directory *identity.Directory
	Book      *tariff.Book
	Store     *store.Store
	Trail     *audit.Trail
	Workspace *workspace.Workspace
	Renderer  *labels.Runner
	Notify    *notify.Queue
	Issuer    *auth.Issuer
	Sessions  *auth.Sessions
	Clock     *clock.Clock
}

// API is one open surface.
type API struct {
	deps   Deps
	router *httpx.Router
	now    func() time.Time
}

// New validates the wiring and registers the routes. A missing dependency is refused here rather than
// panicking on the first request that needed it.
func New(deps Deps) (*API, error) {
	if deps.Config == nil || deps.Directory == nil || deps.Book == nil || deps.Store == nil ||
		deps.Trail == nil || deps.Workspace == nil || deps.Issuer == nil || deps.Sessions == nil {
		return nil, fmt.Errorf("api: incomplete wiring")
	}
	now := time.Now
	if deps.Clock != nil {
		now = deps.Clock.Now
	}
	a := &API{deps: deps, router: httpx.NewRouter(), now: now}
	a.routes()
	return a, nil
}

// routes registers the surface. The route table is the list of things a token can do; a route that is
// not here is not served, whatever its handler does.
func (a *API) routes() {
	r := a.router
	r.Handle(http.MethodGet, "/healthz", a.health)
	r.Handle(http.MethodPost, "/api/events", a.require(auth.ActionWriteEvents, a.ingestEvents))
	r.Handle(http.MethodPost, "/api/shipments", a.require(auth.ActionWriteShipments, a.createShipment))
	r.Handle(http.MethodGet, "/api/shipments", a.require(auth.ActionReadShipments, a.listShipments))
	r.Handle(http.MethodGet, "/api/shipments/{id}", a.require(auth.ActionReadShipments, a.showShipment))
	r.Handle(http.MethodPost, "/api/shipments/{id}/label", a.require(auth.ActionWriteShipments, a.renderLabel))
	r.Handle(http.MethodPost, "/api/close", a.require(auth.ActionWriteClose, a.closeStatement))
	r.Handle(http.MethodGet, "/api/statements", a.require(auth.ActionReadStatements, a.listStatements))
	r.Handle(http.MethodGet, "/api/statements/{period}", a.require(auth.ActionReadStatements, a.showStatement))
	r.Handle(http.MethodPost, "/api/deliveries", a.require(auth.ActionWriteDelivery, a.deliverStatement))
	r.Handle(http.MethodGet, "/api/reports", a.require(auth.ActionReadReports, a.listReports))
	r.Handle(http.MethodGet, "/api/reports/{name}", a.require(auth.ActionReadReports, a.runReport))
	r.Handle(http.MethodGet, "/api/export/{name}", a.require(auth.ActionExportReports, a.exportReport))
	r.Handle(http.MethodGet, "/api/admin/stats", a.require(auth.ActionOperate, a.stats))
	r.Handle(http.MethodGet, "/api/admin/audit", a.require(auth.ActionReadAudit, a.auditLog))
	r.Handle(http.MethodGet, "/api/admin/rates", a.require(auth.ActionOperate, a.rates))
}

// Handler is the surface's HTTP handler, with the middleware a deployment serves behind.
func (a *API) Handler(logger *log.Logger) http.Handler {
	return httpx.Chain(a.router,
		httpx.Recover(logger),
		httpx.SecureHeaders(),
		httpx.WithRequestIdentifier(),
		httpx.Timing(a.now),
		httpx.Logging(logger, a.now),
		a.authenticate,
	)
}

// Routes lists the surface's routes.
func (a *API) Routes() []string { return a.router.Patterns() }

// SetClock replaces the clock responses are stamped with and windows are compared against.
func (a *API) SetClock(now func() time.Time) {
	if now != nil {
		a.now = func() time.Time { return now().UTC() }
	}
}

// principal is who a request acts as: the verified claims and the partner the subject resolved to.
type principal struct {
	claims  auth.Claims
	partner identity.Partner
	tokenID string
}

type principalKey struct{}

// authenticate verifies the request's token and resolves its subject before any handler runs. A request
// to the health route is not authenticated: a probe has no token, and the answer carries no record.
func (a *API) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		token, err := httpx.BearerToken(r)
		if err != nil {
			httpx.JSONError(w, http.StatusUnauthorized, "unauthorized", "a bearer token is required")
			return
		}
		claims, err := a.deps.Issuer.Verify(token, a.now())
		if err != nil {
			httpx.Error(w, err)
			return
		}
		if !a.deps.Config.TenantAllowed(claims.Tenant) {
			httpx.JSONError(w, http.StatusForbidden, "forbidden", "the deployment does not serve this tenant")
			return
		}
		partner, err := a.deps.Directory.Resolve(claims.Tenant, claims.Subject)
		if err != nil {
			// The subject is compared exactly with the directory, so a token for another tenant's partner,
			// or for a partner that does not exist, resolves to nothing and is answered as unauthorized.
			httpx.JSONError(w, http.StatusUnauthorized, "unauthorized", "the token's subject is not known")
			return
		}
		session := a.deps.Sessions.Record(tokenID(claims), claims)
		p := principal{claims: claims, partner: partner, tokenID: session.ID}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}

// require wraps a handler with the policy check. The action is a constant of the package's table, so a
// route cannot ask for a scope the table does not carry and get a handler that runs anyway.
func (a *API) require(action string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := principalOf(r)
		if !ok {
			httpx.JSONError(w, http.StatusUnauthorized, "unauthorized", "a bearer token is required")
			return
		}
		if err := auth.Require(p.claims.Scopes, action); err != nil {
			a.auditRefusal(p, r, action, err)
			httpx.Error(w, err)
			return
		}
		handler(w, r)
	}
}

// health answers the liveness probe: the service is up and can read its configuration.
func (a *API) health(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "shiptrack",
		"tenants": len(a.deps.Config.Tenants),
	})
}

// tokenID names a token's session: a digest of the claims, so the table keeps no credential at rest.
func tokenID(claims auth.Claims) string {
	sum := sha256.Sum256([]byte(claims.Subject + "\x00" + claims.Tenant + "\x00" +
		fmt.Sprint(claims.Expiry) + "\x00" + fmt.Sprint(claims.Scopes)))
	return hex.EncodeToString(sum[:16])
}

// auditRefusal records a refusal a mutating route answered. A read route's refusal is logged and not
// stored: the trail records what changed the service's records, and a refused read changed nothing.
func (a *API) auditRefusal(p principal, r *http.Request, action string, err error) {
	if r.Method == http.MethodGet {
		return
	}
	detail := action + " " + httpx.Describe(err)
	_, _ = a.deps.Trail.Append("request.refused", p.partner.Describe(), detail, a.now())
}

// auditWrite records an action that changed a record. A full trail refuses the entry and the handler
// answers with the error: the record was still written, so the caller must not retry the write, and the
// response says the entry could not be stored.
func (a *API) auditWrite(p principal, kind, detail string) error {
	entry, err := a.deps.Trail.Append(kind, p.partner.Describe(), detail, a.now())
	if err != nil {
		return fmt.Errorf("api: trail: %w", err)
	}
	_ = entry
	return nil
}

// principalOf reads the principal the middleware stored.
func principalOf(r *http.Request) (principal, bool) {
	p, ok := r.Context().Value(principalKey{}).(principal)
	return p, ok
}
