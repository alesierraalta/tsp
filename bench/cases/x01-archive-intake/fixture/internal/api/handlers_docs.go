package api

import (
	"net/http"
	"strconv"

	"example.com/archive/internal/audit"
	"example.com/archive/internal/auth"
	"example.com/archive/internal/export"
	"example.com/archive/internal/httpx"
	"example.com/archive/internal/render"
	"example.com/archive/internal/search"
	"example.com/archive/internal/store"
)

// handleDocuments lists the principal's objects. The listing is always the principal's own tenant,
// even when the query string names another one, and the filter's limit is the deployment's page size
// unless the caller asked for less.
func (a *API) handleDocuments(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.principal(w, r, auth.ActionRead)
	if !ok {
		return
	}
	limit, err := httpx.IntQuery(r, "limit", a.cfg.PageSize, a.cfg.PageSize)
	if err != nil {
		a.refuse(w, r, principal, auth.ActionRead, err.Error())
		return
	}
	filter := store.Filter{
		Tenant: principal.Tenant,
		Kind:   httpx.Query(r, "kind"),
		Batch:  httpx.Query(r, "batch"),
		Query:  httpx.Query(r, "q"),
		Limit:  limit,
	}
	records, err := a.store.List(filter)
	if err != nil {
		a.fail(w, r, principal, auth.ActionRead, err)
		return
	}
	a.record(principal, auth.ActionRead, "/api/documents", audit.OutcomeOK, strconv.Itoa(len(records))+" rows")
	if httpx.WantsHTML(r) {
		httpx.HTML(w, http.StatusOK, render.Index(records, a.clock()))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"documents": documentViews(records),
		"total":     a.store.CountMatching(filter),
		"limit":     limit,
	})
}

// handleDocument serves one object: its metadata as JSON, or its page when the client asked for HTML.
func (a *API) handleDocument(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.principal(w, r, auth.ActionRead)
	if !ok {
		return
	}
	record, ok := a.document(w, r, principal)
	if !ok {
		return
	}
	text, err := a.store.ReadText(record.ID)
	if err != nil {
		a.fail(w, r, principal, auth.ActionRead, err)
		return
	}
	a.record(principal, auth.ActionRead, record.ID, audit.OutcomeOK, "metadata")
	if httpx.WantsHTML(r) {
		httpx.HTML(w, http.StatusOK, render.Object(record, text, a.clock()))
		return
	}
	view := documentView(record)
	view["text_preview"] = render.SummaryLine(text)
	httpx.JSON(w, http.StatusOK, view)
}

// handleDocumentText serves the text the pipeline extracted from one object.
func (a *API) handleDocumentText(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.principal(w, r, auth.ActionRead)
	if !ok {
		return
	}
	record, ok := a.document(w, r, principal)
	if !ok {
		return
	}
	text, err := a.store.ReadText(record.ID)
	if err != nil {
		a.fail(w, r, principal, auth.ActionRead, err)
		return
	}
	a.record(principal, auth.ActionRead, record.ID, audit.OutcomeOK, "text")
	httpx.JSON(w, http.StatusOK, map[string]any{"id": record.ID, "text": text, "bytes": len(text)})
}

// handleDocumentDownload serves the object's container bytes. The content type is pinned to the
// archive's own, so a stored file cannot make a browser execute it.
func (a *API) handleDocumentDownload(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.principal(w, r, auth.ActionRead)
	if !ok {
		return
	}
	record, ok := a.document(w, r, principal)
	if !ok {
		return
	}
	body, err := a.store.Read(record.ID)
	if err != nil {
		a.fail(w, r, principal, auth.ActionRead, err)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+httpx.SafeFilename(record.Name)+"\"")
	a.record(principal, auth.ActionRead, record.ID, audit.OutcomeOK, "download "+strconv.Itoa(len(body))+" bytes")
	httpx.Bytes(w, http.StatusOK, "application/octet-stream", body)
}

// handleDocumentDelete removes one object and its index entry.
func (a *API) handleDocumentDelete(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.principal(w, r, auth.ActionDelete)
	if !ok {
		return
	}
	record, ok := a.document(w, r, principal)
	if !ok {
		return
	}
	if err := a.store.Delete(record.ID); err != nil {
		a.fail(w, r, principal, auth.ActionDelete, err)
		return
	}
	a.index.Remove(record.ID)
	a.record(principal, auth.ActionDelete, record.ID, audit.OutcomeOK, "deleted "+record.Name)
	w.WriteHeader(http.StatusNoContent)
}

// handleSearch answers the token index: the query string is parsed once, the tenant is forced to the
// principal's, and the answer is a ranked list.
func (a *API) handleSearch(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.principal(w, r, auth.ActionRead)
	if !ok {
		return
	}
	raw := map[string]string{}
	for _, key := range []string{"q", "kind", "tenant", "pages_min", "pages_max", "limit", "batch"} {
		if value := httpx.Query(r, key); value != "" {
			raw[key] = value
		}
	}
	query, err := search.ParseQuery(raw, a.cfg.PageSize, a.lim.MaxExportRows)
	if err != nil {
		a.badRequest(w, r, principal, auth.ActionRead, err)
		return
	}
	matches := a.index.Search(query.Text, query.Limit)
	results := make([]map[string]any, 0, len(matches))
	for _, match := range matches {
		if match.Record.Tenant != principal.Tenant {
			continue
		}
		view := documentView(match.Record)
		view["score"] = match.Score
		results = append(results, view)
	}
	a.record(principal, auth.ActionRead, "/api/search", audit.OutcomeOK, strconv.Itoa(len(results))+" hits")
	httpx.JSON(w, http.StatusOK, map[string]any{"query": query.String(), "results": results})
}

// handleExport writes the tenant's objects as CSV. The export needs both the export scope and the read
// scope, which the policy table checks together.
func (a *API) handleExport(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.principal(w, r, auth.ActionExport)
	if !ok {
		return
	}
	records := a.tenantRecords(principal, store.Filter{})
	body, err := export.CSV(records, a.lim.MaxExportRows)
	if err != nil {
		a.fail(w, r, principal, auth.ActionExport, err)
		return
	}
	a.record(principal, auth.ActionExport, "/api/export.csv", audit.OutcomeOK, strconv.Itoa(len(records))+" rows")
	httpx.CSV(w, export.Filename(principal.Tenant, a.clock()), body)
}

// document reads the object the route names and proves it belongs to the principal's tenant. A record
// of another tenant is answered exactly like a missing one.
func (a *API) document(w http.ResponseWriter, r *http.Request, p auth.Principal) (store.Record, bool) {
	id := httpx.Param(r, "id")
	record, err := a.store.Get(id)
	if err != nil {
		a.notFound(w, r, p, auth.ActionRead, "no such document")
		return store.Record{}, false
	}
	if !auth.InTenant(p, record.Tenant) {
		a.notFound(w, r, p, auth.ActionRead, "no such document")
		return store.Record{}, false
	}
	return record, true
}
