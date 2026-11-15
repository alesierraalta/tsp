// Package api is the archive's HTTP surface: the routes, the handlers behind them, the scope each
// route requires, and the audit record every mutating call appends.
package api

import (
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	"example.com/archive/internal/audit"
	"example.com/archive/internal/auth"
	"example.com/archive/internal/clock"
	"example.com/archive/internal/config"
	"example.com/archive/internal/httpx"
	"example.com/archive/internal/intake"
	"example.com/archive/internal/search"
	"example.com/archive/internal/staging"
	"example.com/archive/internal/store"
)

// API is one wired HTTP surface.
type API struct {
	cfg      config.Config
	lim      config.Limits
	store    *store.Store
	runner   *intake.Runner
	staging  *staging.Manager
	issuer   *auth.Issuer
	trail    *audit.Trail
	sessions *auth.Sessions
	index    *search.Index
	now      clock.Clock
	mu       sync.RWMutex
}

// New wires the surface over an open store, an intake runner and a staging tree.
func New(cfg config.Config, s *store.Store, runner *intake.Runner, st *staging.Manager, issuer *auth.Issuer, trail *audit.Trail, now clock.Clock) *API {
	a := &API{
		cfg:      cfg,
		lim:      cfg.Limits(),
		store:    s,
		runner:   runner,
		staging:  st,
		issuer:   issuer,
		trail:    trail,
		sessions: auth.NewSessions(64, now),
		index:    search.New(),
		now:      now,
	}
	for _, record := range s.All() {
		text, err := s.ReadText(record.ID)
		if err != nil {
			text = ""
		}
		a.index.Add(record, text)
	}
	return a
}

// SetClock replaces the clock the handlers stamp responses with and the issuer reads expiration from.
func (a *API) SetClock(now clock.Clock) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.now = now
	a.issuer.SetClock(now)
	a.sessions = auth.NewSessions(64, now)
}

// Trail is the audit trail the surface appends to.
func (a *API) Trail() *audit.Trail { return a.trail }

// Config is the configuration the surface runs with.
func (a *API) Config() config.Config { return a.cfg }

// now reads the clock under the lock, so a test that replaces it mid-run is seen.
func (a *API) clock() time.Time {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return clock.Now(a.now)
}

// Handler returns the HTTP handler of the surface: the router wrapped in the middleware every request
// goes through.
func (a *API) Handler(logger *log.Logger) http.Handler {
	return httpx.Chain(a.router(),
		httpx.Recoverer(logger),
		httpx.Logger(logger),
		httpx.SecurityHeaders(),
		httpx.MaxBody(a.lim.MaxBodyBytes),
	)
}

// Routes lists the endpoint table, in registration order.
func (a *API) Routes() []string { return a.router().Routes() }

// router builds the route table. The table is the one place a route and its scope are written down, so
// a handler cannot be reached without the scope its route declares.
func (a *API) router() *httpx.Router {
	r := httpx.NewRouter()
	r.Get("/healthz", a.handleHealth)
	r.Get("/api/documents", a.handleDocuments)
	r.Get("/api/documents/{id}", a.handleDocument)
	r.Get("/api/documents/{id}/text", a.handleDocumentText)
	r.Get("/api/documents/{id}/download", a.handleDocumentDownload)
	r.Delete("/api/documents/{id}", a.handleDocumentDelete)
	r.Get("/api/search", a.handleSearch)
	r.Get("/api/export.csv", a.handleExport)

	r.Post("/api/batches", a.handleBatchCreate)
	r.Get("/api/batches", a.handleBatchList)
	r.Get("/api/batches/{id}", a.handleBatch)
	r.Post("/api/batches/{id}/resume", a.handleBatchResume)
	r.Delete("/api/batches/{id}", a.handleBatchAbort)
	r.Get("/api/batches/{id}/report", a.handleBatchReport)
	r.Get("/api/batches/{id}/pages/{page}", a.handleBatchPage)
	r.Get("/api/batches/{id}/files", a.handleStagedListing)

	r.Get("/api/admin/stats", a.handleStats)
	r.Get("/api/admin/audit", a.handleAudit)
	r.Get("/api/admin/console", a.handleConsole)
	r.Get("/api/admin/verify", a.handleVerify)
	r.Post("/api/admin/sweep", a.handleSweep)
	return r
}

// principal verifies the request's token, checks the tenant is one this deployment serves, and checks
// the action's scope. Every handler begins with it, and a refusal is audited like any other outcome.
func (a *API) principal(w http.ResponseWriter, r *http.Request, action string) (auth.Principal, bool) {
	token, ok := auth.Bearer(r.Header.Get("Authorization"))
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "missing bearer token")
		return auth.Principal{}, false
	}
	principal, err := a.issuer.Verify(token)
	if err != nil {
		a.trail.Record("unknown", "", action, r.URL.Path, audit.OutcomeRefused, "invalid token")
		httpx.Fail(w, http.StatusUnauthorized, "invalid token")
		return auth.Principal{}, false
	}
	if !auth.InAnyTenant(principal, a.cfg.Tenants) {
		a.trail.Record(principal.Subject, principal.Tenant, action, r.URL.Path, audit.OutcomeRefused, "tenant not served")
		httpx.Fail(w, http.StatusForbidden, "tenant not served")
		return auth.Principal{}, false
	}
	if err := auth.Check(principal, action); err != nil {
		a.trail.Record(principal.Subject, principal.Tenant, action, r.URL.Path, audit.OutcomeRefused, err.Error())
		httpx.Fail(w, http.StatusForbidden, "forbidden")
		return auth.Principal{}, false
	}
	return principal, true
}

// record appends one audit entry for a handler's outcome.
func (a *API) record(p auth.Principal, action, target, outcome, detail string) {
	a.trail.Record(p.Subject, p.Tenant, action, target, outcome, detail)
}

// documentView is the JSON shape one object is served as. It is written once, so a listing row and a
// single-object response carry the same fields.
func documentView(record store.Record) map[string]any {
	view := map[string]any{
		"id":        record.ID,
		"tenant":    record.Tenant,
		"name":      record.Name,
		"title":     record.Title,
		"kind":      record.Kind,
		"pages":     record.Pages,
		"bytes":     record.Bytes,
		"digest":    record.Digest,
		"batch":     record.Batch,
		"entry":     record.Entry,
		"created":   record.Created,
		"committed": record.Commit,
	}
	if len(record.Tags) > 0 {
		view["tags"] = record.Tags
	}
	if record.Source != "" {
		view["source"] = record.Source
	}
	return view
}

// documentViews renders a listing.
func documentViews(records []store.Record) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, documentView(record))
	}
	return out
}

// tenantRecords lists the records the principal may read: its own tenant's, nothing else. A listing
// never crosses a tenant, whatever the query asked for.
func (a *API) tenantRecords(p auth.Principal, f store.Filter) []store.Record {
	f.Tenant = p.Tenant
	records, err := a.store.List(f)
	if err != nil {
		return nil
	}
	return records
}

// sortedKeys is a small helper the admin pages use to print a count map in a stable order.
func sortedKeys(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
