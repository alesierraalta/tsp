package httpapi

import (
	"fmt"
	"net/http"
	"strconv"

	"example.com/billing/internal/auth"
	"example.com/billing/internal/httpx"
	"example.com/billing/internal/reports"
)

// listReports answers `GET /api/reports` with the catalog: what each report answers and the filters it
// accepts. The catalog is the contract a client reads before it sends a filter, so a parameter a
// report does not declare is visible before the request is made.
func (a *API) listReports(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if _, err := tenantOf(principal); err != nil {
		httpx.FailError(w, err)
		return
	}
	catalog := a.reports.Catalog()
	httpx.Write(w, http.StatusOK, map[string]any{
		"reports": catalog, "count": len(catalog),
	})
}

// runReport answers `GET /api/reports/{name}`. The filters the caller sends are read through the
// report's declaration: a parameter the report does not declare is refused rather than ignored, and
// every value is bound as an argument of the statement.
func (a *API) runReport(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	tenant, err := tenantOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	name := httpx.Param(r, "name")
	filters, limit, err := filtersOf(r)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	answer, err := a.reports.Run(a.store, name, tenant, filters, limit)
	if err != nil {
		if _, lookup := a.reports.Get(name); lookup != nil {
			httpx.FailError(w, httpx.NotFoundError("no such report"))
			return
		}
		httpx.FailError(w, httpx.BadRequest(err.Error()))
		return
	}
	httpx.Write(w, http.StatusOK, answer)
}

// exportReport answers `GET /api/reports/{name}/export.csv`. The usage report is the one a deployment
// exports; a name it does not export is a not-found rather than an empty file, so a client that asked
// for the wrong report is told.
func (a *API) exportReport(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	tenant, err := tenantOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	name := httpx.Param(r, "name")
	if name != "usage" {
		httpx.FailError(w, httpx.NotFoundError("no such export"))
		return
	}
	status := r.URL.Query().Get("status")
	body, err := reports.ExportCSV(a.store, tenant, status)
	if err != nil {
		httpx.FailError(w, httpx.BadRequest(err.Error()))
		return
	}
	httpx.Attachment(w, "text/csv", reports.ExportName(tenant), body)
}

// filtersOf reads the query parameters a report request carries: every parameter except the row bound
// is a filter, and nothing else is passed through as a statement fragment.
func filtersOf(r *http.Request) (map[string]string, int, error) {
	values := r.URL.Query()
	filters := make(map[string]string, len(values))
	limit := 0
	for name := range values {
		value := values.Get(name)
		if name == "limit" {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed <= 0 {
				return nil, 0, httpx.BadRequest(fmt.Sprintf("limit %q is not a positive number", value))
			}
			limit = parsed
			continue
		}
		if name == "cursor" {
			continue
		}
		filters[name] = value
	}
	return filters, limit, nil
}
