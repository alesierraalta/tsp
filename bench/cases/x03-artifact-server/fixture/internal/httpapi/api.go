// Package httpapi is the artifact server's HTTP surface: the routes, the handlers behind them, the
// scope each route requires and the audit record every mutating call appends.
package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"example.com/artifacts/internal/artifacts"
	"example.com/artifacts/internal/audit"
	"example.com/artifacts/internal/auth"
	"example.com/artifacts/internal/clock"
	"example.com/artifacts/internal/config"
	"example.com/artifacts/internal/httpx"
	"example.com/artifacts/internal/notify"
	"example.com/artifacts/internal/packaging"
	"example.com/artifacts/internal/storage"
)

// Deps are the values the surface is wired with.
type Deps struct {
	Config          config.Config
	Store           *artifacts.Store
	Layout          *storage.Layout
	Packer          *packaging.Packer
	Relay           *notify.Relay
	Issuer          *auth.Issuer
	Policy          *auth.Policy
	Trail           *audit.Trail
	Subscriptions   func(project string) []notify.Subscription
	AddSubscription func(notify.Subscription) error
	Now             clock.Clock
}

// API is one wired HTTP surface.
type API struct {
	cfg    config.Config
	store  *artifacts.Store
	layout *storage.Layout
	packer *packaging.Packer
	relay  *notify.Relay
	issuer *auth.Issuer
	policy *auth.Policy
	trail  *audit.Trail
	subs   func(project string) []notify.Subscription
	addSub func(notify.Subscription) error
	router *httpx.Router
	mu     sync.RWMutex
	now    clock.Clock
}

// New validates the dependencies and registers the routes.
func New(deps Deps) (*API, error) {
	if err := deps.Config.Validate(); err != nil {
		return nil, err
	}
	if deps.Store == nil || deps.Layout == nil || deps.Packer == nil || deps.Relay == nil ||
		deps.Issuer == nil || deps.Policy == nil || deps.Trail == nil || deps.Subscriptions == nil {
		return nil, errors.New("httpapi: the surface needs a store, a layout, a packer, a relay, an issuer, a policy, a trail and the subscriptions")
	}
	now := deps.Now
	if now == nil {
		now = clock.System()
	}
	a := &API{
		cfg: deps.Config, store: deps.Store, layout: deps.Layout, packer: deps.Packer,
		relay: deps.Relay, issuer: deps.Issuer, policy: deps.Policy, trail: deps.Trail,
		subs: deps.Subscriptions, addSub: deps.AddSubscription, now: now,
	}
	a.router = httpx.NewRouter()
	a.register()
	return a, nil
}

// SetClock replaces the surface's clock.
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

// ServeHTTP routes a request.
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) { a.router.ServeHTTP(w, r) }

// Patterns lists the registered routes.
func (a *API) Patterns() []string { return a.router.Patterns() }

// handlerFunc is a handler with the principal the middleware resolved.
type handlerFunc func(http.ResponseWriter, *http.Request, auth.Principal)

// register writes the route table.
func (a *API) register() {
	a.router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.Write(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	a.router.Get("/api/artifacts", a.route("artifacts.read", a.listArtifacts))
	a.router.Get("/api/artifacts/{id}", a.route("artifacts.read", a.getArtifact))
	a.router.Post("/api/artifacts", a.route("artifacts.write", a.uploadArtifact))
	a.router.Post("/api/artifacts/{id}/pack", a.route("artifacts.write", a.packArtifact))
	a.router.Post("/api/artifacts/{id}/repackage", a.route("artifacts.write", a.repackageArtifact))
	a.router.Delete("/api/artifacts/{id}", a.route("artifacts.write", a.deleteArtifact))
	a.router.Get("/api/artifacts/{id}/content", a.route("artifacts.read", a.downloadArtifact))

	a.router.Get("/api/jobs", a.route("jobs.read", a.listJobs))
	a.router.Get("/api/jobs/{id}", a.route("jobs.read", a.getJob))

	a.router.Get("/api/subscriptions", a.route("webhooks.read", a.listSubscriptions))
	a.router.Post("/api/subscriptions", a.route("webhooks.write", a.createSubscription))
	a.router.Post("/api/subscriptions/{id}/deliver", a.route("webhooks.write", a.redeliver))
	a.router.Post("/api/events/broadcast", a.route("webhooks.write", a.broadcast))

	a.router.Get("/api/admin/console", a.route("admin.operate", a.console))
	a.router.Get("/api/admin/audit", a.route("audit.read", a.auditTrail))
	a.router.Post("/api/admin/sweep", a.route("admin.operate", a.sweep))
}

// route registers a route behind the token check, the policy check and the rate limit.
func (a *API) route(action string, handler handlerFunc) http.HandlerFunc {
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
		handler(w, r, principal)
	}
}

// authenticate reads the bearer token of a request and resolves the principal it names.
func (a *API) authenticate(r *http.Request) (auth.Principal, error) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return auth.Principal{}, httpx.Unauthorized("the request carries no token")
	}
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
		return auth.Principal{}, httpx.Unauthorized("the authorization header is not a bearer token")
	}
	claims, err := a.issuer.Verify(strings.TrimSpace(token))
	if err != nil {
		if errors.Is(err, auth.ErrExpired) {
			return auth.Principal{}, httpx.Unauthorized("the token has expired")
		}
		return auth.Principal{}, httpx.Unauthorized("the token is not one this service issued")
	}
	if !a.cfg.HasProject(claims.Project) {
		return auth.Principal{}, httpx.Forbidden("the token names a project this deployment does not serve")
	}
	return auth.NewPrincipal(claims, a.isOperator(claims.Subject))
}

// isOperator reports whether a subject is one the deployment names as its operator.
func (a *API) isOperator(subject string) bool {
	for _, name := range a.cfg.Tools {
		if name == "operator:"+subject {
			return true
		}
	}
	return false
}

// record appends one trail entry.
func (a *API) record(principal auth.Principal, action, target, outcome, detail string) {
	subject := principal.Subject
	if subject == "" {
		subject = "(anonymous)"
	}
	_, _ = a.trail.Append(audit.Record{
		At: a.nowUTC(), Project: principal.Project, Subject: subject,
		Action: action, Target: target, Outcome: outcome, Detail: detail,
	})
}

// nowUTC returns the surface's current instant.
func (a *API) nowUTC() time.Time { return a.Clock()().UTC() }

// targetOf returns the identifier a request is aimed at.
func targetOf(r *http.Request) string {
	if id := httpx.Param(r, "id"); id != "" {
		return id
	}
	return ""
}

// projectOf returns the principal's project.
func projectOf(principal auth.Principal) (string, error) {
	if principal.Project == "" {
		return "", httpx.Unauthorized("the token carries no project")
	}
	return principal.Project, nil
}
