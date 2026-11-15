// Package httpapi is the HTTP surface of the service: the routes, the handlers and the mapping from
// a service error to a status code.
package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"example.com/insight/internal/access"
	"example.com/insight/internal/reports"
	"example.com/insight/internal/store"
)

// API is one mounted service.
type API struct {
	store    *store.Store
	registry *reports.Registry
	policy   *access.Policy
	resolver *access.Resolver
	issuer   *access.Issuer
	mux      *http.ServeMux
	now      func() time.Time
	requests *atomic.Int64
}

// New wires the HTTP surface over the store, the directory and the token issuer.
func New(st *store.Store, registry *reports.Registry, dir *access.Directory, issuer *access.Issuer) *API {
	a := &API{
		store:    st,
		registry: registry,
		policy:   access.NewPolicy(),
		resolver: access.NewResolver(dir),
		issuer:   issuer,
		mux:      http.NewServeMux(),
		now:      func() time.Time { return time.Now().UTC() },
		requests: &atomic.Int64{},
	}
	a.routes()
	return a
}

// SetClock replaces the clock the responses are stamped with.
func (a *API) SetClock(now func() time.Time) {
	if now != nil {
		a.now = now
	}
}

// Routes lists the patterns the service serves.
func (a *API) Routes() []string {
	return []string{
		"GET /healthz",
		"GET /api/reports",
		"GET /api/reports/{name}",
		"GET /api/reports/{name}/export.csv",
		"GET /api/events",
		"GET /api/accounts",
		"GET /api/quota",
	}
}

func (a *API) routes() {
	a.mux.HandleFunc("GET /healthz", a.handleHealth)
	a.mux.HandleFunc("GET /api/reports", a.handleCatalogue)
	a.mux.HandleFunc("GET /api/reports/{name}", a.handleReport)
	a.mux.HandleFunc("GET /api/reports/{name}/export.csv", a.handleExport)
	a.mux.HandleFunc("GET /api/events", a.handleEvents)
	a.mux.HandleFunc("GET /api/accounts", a.handleAccounts)
	a.mux.HandleFunc("GET /api/quota", a.handleQuota)
}

// Handler returns the mux wrapped in the middleware every request passes through.
func (a *API) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.requests.Add(1)
		defer func() {
			if v := recover(); v != nil {
				fail(w, http.StatusInternalServerError, fmt.Sprintf("internal error: %v", v))
			}
		}()
		w.Header().Set("X-Insight-Request", fmt.Sprintf("%d", a.requests.Load()))
		a.mux.ServeHTTP(w, r)
	})
}

// principal resolves the caller of a request and checks one action against the policy. It returns
// the access error the caller should be answered with, so a handler that ignores the error cannot
// continue with an empty principal by accident.
func (a *API) principal(r *http.Request, action string) (access.Principal, error) {
	raw, err := access.Bearer(r.Header.Get("Authorization"))
	if err != nil {
		return access.Principal{}, err
	}
	tok, err := a.issuer.Verify(raw)
	if err != nil {
		return access.Principal{}, err
	}
	pr, err := a.resolver.Resolve(tok)
	if err != nil {
		return access.Principal{}, err
	}
	if err := a.policy.Check(pr, action); err != nil {
		return access.Principal{}, err
	}
	return pr, nil
}

// writeJSON writes one JSON response.
func writeJSON(w http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		fail(w, http.StatusInternalServerError, "cannot encode response")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write(body)
	w.Write([]byte("\n"))
}

// fail writes the standard error body.
func fail(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// failAccess maps an access or report error to a status.
func failAccess(w http.ResponseWriter, err error) {
	status := access.StatusCode(err)
	if status == 0 {
		status = http.StatusBadRequest
	}
	fail(w, status, err.Error())
}
