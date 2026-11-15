package api

import (
	"errors"
	"fmt"
	"net/http"

	"example.com/archive/internal/auth"
	"example.com/archive/internal/httpx"
	"example.com/archive/internal/render"
	"example.com/archive/internal/staging"
	"example.com/archive/internal/store"
)

// fail maps an error to the status and message the client gets. The detail goes to the audit trail, not
// to the response: a refusal names the reason an operator needs, and the response names nothing an
// attacker can use to learn the tree.
func (a *API) fail(w http.ResponseWriter, r *http.Request, p auth.Principal, action string, err error) {
	switch {
	case err == nil:
		httpx.Fail(w, http.StatusInternalServerError, "internal error")
	case errors.Is(err, store.ErrNotFound), errors.Is(err, staging.ErrMissing):
		a.record(p, action, r.URL.Path, "not-found", err.Error())
		httpx.Fail(w, http.StatusNotFound, "no such object")
	case errors.Is(err, staging.ErrOutside):
		a.record(p, action, r.URL.Path, "refused", err.Error())
		httpx.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, staging.ErrInvalid), errors.Is(err, store.ErrInvalid):
		a.record(p, action, r.URL.Path, "refused", err.Error())
		httpx.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, staging.ErrExists), errors.Is(err, store.ErrExists):
		a.record(p, action, r.URL.Path, "refused", err.Error())
		httpx.Fail(w, http.StatusConflict, "already exists")
	case errors.Is(err, store.ErrNotFound):
		a.record(p, action, r.URL.Path, "not-found", err.Error())
		httpx.Fail(w, http.StatusNotFound, "no such object")
	default:
		a.record(p, action, r.URL.Path, "failed", err.Error())
		httpx.Fail(w, http.StatusInternalServerError, "internal error")
	}
}

// refuse records a refusal the handler decided itself, such as a query value it will not read.
func (a *API) refuse(w http.ResponseWriter, r *http.Request, p auth.Principal, action string, message string) {
	a.record(p, action, r.URL.Path, "refused", message)
	httpx.Fail(w, http.StatusBadRequest, message)
}

// notFound answers a request for an object the tenant does not carry. It is the same answer a tenant
// gets for an object of another tenant, so a probe cannot tell the two apart.
func (a *API) notFound(w http.ResponseWriter, r *http.Request, p auth.Principal, action, what string) {
	a.record(p, action, r.URL.Path, "not-found", what)
	if httpx.WantsHTML(r) {
		httpx.HTML(w, http.StatusNotFound, render.NotFound(what))
		return
	}
	httpx.Fail(w, http.StatusNotFound, what)
}

// badRequest is the answer to a request the surface will not read: a malformed id, an unparsed number,
// an unknown filter. The message is a sentence the client can act on.
func (a *API) badRequest(w http.ResponseWriter, r *http.Request, p auth.Principal, action string, err error) {
	a.record(p, action, r.URL.Path, "refused", err.Error())
	httpx.Fail(w, http.StatusBadRequest, err.Error())
}

// pageCap is the largest page number a batch page request may name. It is the deployment's page cap, so
// a request that asks for a page beyond it is refused before the tree is asked for the file.
func (a *API) pageCap() int { return a.lim.MaxPages }

// paramInt reads a positive integer from the route's parameters.
func paramInt(r *http.Request, name string) (int, error) {
	raw := httpx.Param(r, name)
	if raw == "" {
		return 0, fmt.Errorf("missing %s", name)
	}
	n := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("%s=%q is not a number", name, raw)
		}
		n = n*10 + int(r-'0')
		if n > 1_000_000 {
			return 0, fmt.Errorf("%s=%q is out of range", name, raw)
		}
	}
	if n < 1 {
		return 0, fmt.Errorf("%s=%q is not positive", name, raw)
	}
	return n, nil
}
