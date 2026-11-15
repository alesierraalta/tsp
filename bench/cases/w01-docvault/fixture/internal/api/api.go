// Package api is the HTTP surface of docvault: the routes, the handlers and the error mapping
// between the service packages and status codes.
package api

import (
	"errors"
	"log"
	"net/http"
	"time"

	"example.com/docvault/internal/audit"
	"example.com/docvault/internal/auth"
	"example.com/docvault/internal/config"
	"example.com/docvault/internal/httpx"
	"example.com/docvault/internal/vault"
)

// API is one mounted service: configuration, the vault it serves, and the policy every handler
// asks before it touches the tree.
type API struct {
	cfg      config.Config
	limits   config.Limits
	vault    *vault.Vault
	policy   *auth.Policy
	resolver *auth.Resolver
	trail    *audit.Trail
	router   *httpx.Router
	now      func() time.Time
}

// New wires an API over an opened vault and a token issuer.
func New(cfg config.Config, v *vault.Vault, issuer *auth.Issuer) *API {
	a := &API{
		cfg:      cfg,
		limits:   cfg.Limits(),
		vault:    v,
		policy:   auth.NewPolicy(),
		resolver: auth.NewResolver(issuer, auth.NewPolicy()),
		trail:    audit.New(),
		router:   httpx.NewRouter(),
		now:      func() time.Time { return time.Now().UTC() },
	}
	a.routes()
	return a
}

// SetClock replaces the clock the handlers stamp responses with.
func (a *API) SetClock(now func() time.Time) {
	if now != nil {
		a.now = now
	}
}

// Trail is the audit trail of the instance.
func (a *API) Trail() *audit.Trail { return a.trail }

// Vault is the tree the instance serves.
func (a *API) Vault() *vault.Vault { return a.vault }

// Handler returns the router wrapped in the middleware every request passes through.
func (a *API) Handler(logger *log.Logger) http.Handler {
	router := a.router
	return httpx.Chain(router,
		httpx.RequestID,
		httpx.Recover(logger),
		httpx.AccessLog(logger),
		httpx.MaxBody(a.limits.MaxBody),
	)
}

// Routes lists what the instance serves.
func (a *API) Routes() []string { return a.router.Routes() }

func (a *API) routes() {
	a.router.Get("/healthz", httpx.Health(a.router.Routes(), a.now))

	a.router.Get("/api/documents", a.handleList)
	a.router.Post("/api/documents", a.handleCreate)
	a.router.Get("/api/documents/{id}", a.handleGet)
	a.router.Post("/api/documents/{id}", a.handleUpdate)
	a.router.Handle(http.MethodDelete, "/api/documents/{id}", a.handleDelete)

	a.router.Get("/api/documents/{id}/preview", a.handlePreview)
	a.router.Get("/api/documents/{id}/download", a.handleDownload)
	a.router.Get("/api/documents/{id}/thumbnail", a.handleThumbnail)
	a.router.Get("/api/documents/{id}/object", a.handleObject)
	a.router.Get("/api/attachments", a.handleAttachments)
	a.router.Get("/api/attachments/{name}", a.handleAttachment)
	a.router.Post("/api/attachments/{name}", a.handleAttachmentUpload)

	a.router.Get("/api/export.csv", a.handleExport)
	a.router.Get("/api/admin/stats", a.handleStats)
	a.router.Post("/api/admin/reindex", a.handleReindex)
	a.router.Post("/api/admin/purge", a.handlePurge)
	a.router.Get("/api/admin/audit", a.handleAudit)
}

// principal resolves the session of a request and checks one action against the policy. It writes
// the refusal itself and returns ok=false, so a handler cannot forget to return on refusal and
// cannot read the vault after one.
func (a *API) principal(w http.ResponseWriter, r *http.Request, action string) (auth.Principal, bool) {
	sess, err := a.resolver.Resolve(r)
	if err != nil {
		httpx.Fail(w, auth.StatusCode(err), err.Error())
		return auth.Principal{}, false
	}
	pr := sess.Principal
	if err := a.policy.Check(pr, action); err != nil {
		a.trail.Record(pr.Subject, pr.Tenant, action, "", audit.OutcomeRefused, err.Error())
		httpx.Fail(w, http.StatusForbidden, err.Error())
		return auth.Principal{}, false
	}
	if !a.cfg.TenantAllowed(pr.Tenant) {
		a.trail.Record(pr.Subject, pr.Tenant, action, "", audit.OutcomeRefused, "unknown tenant")
		httpx.Fail(w, http.StatusForbidden, "unknown tenant")
		return auth.Principal{}, false
	}
	return pr, true
}

// fail maps a service error to the status the API answers with.
func (a *API) fail(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, vault.ErrOutside), errors.Is(err, vault.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, vault.ErrNoObject), errors.Is(err, vault.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, vault.ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, auth.ErrForbidden):
		status = http.StatusForbidden
	}
	httpx.Fail(w, status, err.Error())
}

// docOf looks up the metadata of the document a request names.
func (a *API) docOf(r *http.Request) (vault.Doc, error) {
	return a.vault.Store().ByID(httpx.Param(r, "id"))
}
