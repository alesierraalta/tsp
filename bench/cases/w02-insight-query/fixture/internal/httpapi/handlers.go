package httpapi

import (
	"net/http"

	"example.com/insight/internal/access"
	"example.com/insight/internal/query"
	"example.com/insight/internal/reports"
	"example.com/insight/internal/store"
)

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"at":     a.now().Format("2006-01-02T15:04:05Z07:00"),
		"routes": len(a.Routes()),
	})
}

// handleCatalogue lists the reports the service serves.
func (a *API) handleCatalogue(w http.ResponseWriter, r *http.Request) {
	if _, err := a.principal(r, access.ActionRead); err != nil {
		failAccess(w, err)
		return
	}
	defs := a.registry.Definitions()
	out := make([]map[string]any, 0, len(defs))
	for _, def := range defs {
		out = append(out, reports.Describe(def))
	}
	writeJSON(w, http.StatusOK, map[string]any{"reports": out})
}

// handleReport runs one report for the caller's tenant.
func (a *API) handleReport(w http.ResponseWriter, r *http.Request) {
	pr, err := a.principal(r, access.ActionRead)
	if err != nil {
		failAccess(w, err)
		return
	}
	def, err := a.registry.Lookup(r.PathValue("name"))
	if err != nil {
		fail(w, http.StatusNotFound, err.Error())
		return
	}
	params := map[string]string{}
	sent := map[string]string{}
	for key, values := range r.URL.Query() {
		sent[key] = values[0]
	}
	if err := query.Known(def.Filters, sent); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	for _, filter := range def.Filters {
		if value := sent[filter.Param]; value != "" {
			params[filter.Param] = value
		}
	}
	result, err := reports.Run(a.store.DB(), def, pr.Tenant, params)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	rows := reports.Rows(result)
	writeJSON(w, http.StatusOK, map[string]any{
		"report":  def.Name,
		"tenant":  pr.Tenant,
		"columns": result.Columns,
		"rows":    rows,
		"count":   len(rows),
	})
}

// handleEvents runs the usage report with the status filter, which is the endpoint the console
// polls. The status is validated against the store's list before the statement is built.
func (a *API) handleEvents(w http.ResponseWriter, r *http.Request) {
	pr, err := a.principal(r, access.ActionRead)
	if err != nil {
		failAccess(w, err)
		return
	}
	def, err := a.registry.Lookup("usage")
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	params := map[string]string{}
	if status := r.URL.Query().Get("status"); status != "" {
		if !store.IsStatus(status) {
			fail(w, http.StatusBadRequest, "store: status "+status+" is not accepted")
			return
		}
		params["status"] = status
	}
	result, err := reports.Run(a.store.DB(), def, pr.Tenant, params)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant": pr.Tenant,
		"rows":   reports.Rows(result),
		"count":  len(result.Rows),
	})
}

// handleExport renders the caller's events as CSV.
func (a *API) handleExport(w http.ResponseWriter, r *http.Request) {
	pr, err := a.principal(r, access.ActionExport)
	if err != nil {
		failAccess(w, err)
		return
	}
	body, err := reports.ExportCSV(a.store.DB(), pr.Tenant, r.URL.Query().Get("status"))
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"events.csv\"")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(body))
}

// handleAccounts lists the accounts of the caller's tenant, as the directory carries them.
func (a *API) handleAccounts(w http.ResponseWriter, r *http.Request) {
	pr, err := a.principal(r, access.ActionAccounts)
	if err != nil {
		failAccess(w, err)
		return
	}
	result, err := a.store.DB().Query(
		"SELECT subject, tenant, display, role FROM accounts WHERE tenant = ? ORDER BY subject",
		pr.Tenant,
	)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant":   pr.Tenant,
		"accounts": reports.Rows(result),
	})
}

// handleQuota reports the quota rows of the caller's tenant.
func (a *API) handleQuota(w http.ResponseWriter, r *http.Request) {
	pr, err := a.principal(r, access.ActionQuota)
	if err != nil {
		failAccess(w, err)
		return
	}
	result, err := a.store.DB().Query(
		"SELECT tenant, account, limit, used FROM quotas WHERE tenant = ? ORDER BY account",
		pr.Tenant,
	)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant": pr.Tenant,
		"rows":   reports.Rows(result),
		"count":  len(result.Rows),
	})
}
