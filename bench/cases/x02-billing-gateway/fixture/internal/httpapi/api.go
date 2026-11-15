// Package httpapi is the billing gateway's HTTP surface: the routes, the handlers behind them, the
// scope each route requires and the audit record every mutating call appends. A handler receives the
// principal the middleware resolved and reads the tenant from it, never from a body or a query.
package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"example.com/billing/internal/audit"
	"example.com/billing/internal/auth"
	"example.com/billing/internal/clock"
	"example.com/billing/internal/config"
	"example.com/billing/internal/httpx"
	"example.com/billing/internal/ratelimit"
	"example.com/billing/internal/reports"
	"example.com/billing/internal/store"
	"example.com/billing/internal/tax"
)

// Deps are the values the surface is wired with. Every one is required: a surface built without a trail
// or a policy would serve requests nobody can audit or refuse.
type Deps struct {
	Config  config.Config
	Store   *store.Store
	Issuer  *auth.Issuer
	Policy  *auth.Policy
	Trail   *audit.Trail
	Reports *reports.Registry
	Tax     *tax.Table
	Limiter *ratelimit.Limiter
	Now     clock.Clock
}

// API is one wired HTTP surface.
type API struct {
	cfg     config.Config
	lim     config.Limits
	store   *store.Store
	issuer  *auth.Issuer
	policy  *auth.Policy
	trail   *audit.Trail
	reports *reports.Registry
	tax     *tax.Table
	limiter *ratelimit.Limiter
	router  *httpx.Router
	mu      sync.RWMutex
	now     clock.Clock
}

// New validates the dependencies and registers the routes.
func New(deps Deps) (*API, error) {
	if err := deps.Config.Validate(); err != nil {
		return nil, err
	}
	if deps.Store == nil || deps.Issuer == nil || deps.Policy == nil || deps.Trail == nil ||
		deps.Reports == nil || deps.Tax == nil {
		return nil, fmt.Errorf("httpapi: the surface needs a store, an issuer, a policy, a trail, a registry and a tax table")
	}
	now := deps.Now
	if now == nil {
		now = clock.System()
	}
	a := &API{
		cfg:     deps.Config,
		lim:     deps.Config.Limits(),
		store:   deps.Store,
		issuer:  deps.Issuer,
		policy:  deps.Policy,
		trail:   deps.Trail,
		reports: deps.Reports,
		tax:     deps.Tax,
		limiter: deps.Limiter,
		now:     now,
	}
	a.router = httpx.NewRouter()
	a.register()
	return a, nil
}

// SetClock replaces the clock the surface reads for handlers that stamp an instant.
func (a *API) SetClock(now clock.Clock) {
	if now == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.now = now
}

// Clock returns the surface's clock.
func (a *API) Clock() clock.Clock {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.now
}

// ServeHTTP routes a request through the middleware chain and the handler.
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.router.ServeHTTP(w, r)
}

// Patterns lists the registered routes, for the console page and the tests.
func (a *API) Patterns() []string { return a.router.Patterns() }

// register writes the route table. Each route names the action it performs; the middleware resolves
// the principal and refuses an action the policy does not grant before the handler runs.
func (a *API) register() {
	a.router.Get("/healthz", a.open(func(w http.ResponseWriter, r *http.Request) {
		httpx.Write(w, http.StatusOK, map[string]string{"status": "ok"})
	}))

	a.router.Get("/api/plans", a.route(auth.ActionReadBilling, a.listPlans))
	a.router.Get("/api/accounts", a.route(auth.ActionReadBilling, a.listAccounts))
	a.router.Get("/api/subscriptions", a.route(auth.ActionReadBilling, a.listSubscriptions))
	a.router.Get("/api/subscriptions/{id}", a.route(auth.ActionReadBilling, a.getSubscription))
	a.router.Post("/api/subscriptions/{id}/seats", a.route(auth.ActionWriteBilling, a.changeSeats))
	a.router.Post("/api/subscriptions/{id}/cancel", a.route(auth.ActionWriteBilling, a.cancelSubscription))

	a.router.Get("/api/invoices", a.route(auth.ActionReadInvoices, a.listInvoices))
	a.router.Get("/api/invoices/{id}", a.route(auth.ActionReadInvoices, a.getInvoice))
	a.router.Post("/api/invoices/{id}/payments", a.route(auth.ActionWriteInvoices, a.recordPayment))

	a.router.Post("/api/usage", a.route(auth.ActionWriteUsage, a.recordUsage))

	a.router.Get("/api/reports", a.route(auth.ActionReadReports, a.listReports))
	a.router.Get("/api/reports/{name}", a.route(auth.ActionReadReports, a.runReport))
	a.router.Get("/api/reports/{name}/export.csv", a.route(auth.ActionExportReport, a.exportReport))

	a.router.Get("/api/admin/console", a.route(auth.ActionOperate, a.console))
	a.router.Get("/api/admin/audit", a.route(auth.ActionReadAudit, a.auditTrail))
	a.router.Post("/api/admin/sweep", a.route(auth.ActionOperate, a.sweep))
}

// handlerFunc is a handler that has the principal the middleware resolved.
type handlerFunc func(http.ResponseWriter, *http.Request, auth.Principal)

// open registers a route that needs no token.
func (a *API) open(handler http.HandlerFunc) http.HandlerFunc { return handler }

// route registers a route behind the token check, the policy check and the subject's allowance.
func (a *API) route(action auth.Action, handler handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, err := a.authenticate(r)
		if err != nil {
			httpx.FailError(w, err)
			a.record(auth.Principal{}, action, "", audit.OutcomeRefused, err.Error())
			return
		}
		if err := principal.May(a.policy, action); err != nil {
			a.record(principal, action, targetOf(r), audit.OutcomeRefused, err.Error())
			httpx.FailError(w, httpx.Forbidden(err.Error()))
			return
		}
		if a.limiter != nil {
			if allowed, wait := a.limiter.Allow(principal.Subject); !allowed {
				w.Header().Set("Retry-After", fmt.Sprint(int(wait.Seconds())))
				httpx.FailError(w, httpx.RateLimited(fmt.Sprintf("%q is above its allowance", principal.Subject)))
				return
			}
		}
		handler(w, r, principal)
	}
}

// record appends one trail entry.
func (a *API) record(principal auth.Principal, action auth.Action, target, outcome, detail string) {
	tenant := principal.Tenant
	subject := principal.Subject
	if subject == "" {
		subject = "(anonymous)"
	}
	_, _ = a.trail.Append(audit.Record{
		At: a.Clock()().UTC(), Tenant: tenant, Subject: subject,
		Action: string(action), Target: target, Outcome: outcome, Detail: detail,
	})
}

// targetOf returns the identifier a request is aimed at, for the audit record.
func targetOf(r *http.Request) string {
	if id := httpx.Param(r, "id"); id != "" {
		return id
	}
	if name := httpx.Param(r, "name"); name != "" {
		return name
	}
	return ""
}

// now returns the surface's current instant.
func (a *API) nowUTC() time.Time { return a.Clock()().UTC() }

// errors the surface maps to statuses.
var (
	// errNoTenant is returned when a principal carries no tenant.
	errNoTenant = errors.New("the token carries no tenant")
)
