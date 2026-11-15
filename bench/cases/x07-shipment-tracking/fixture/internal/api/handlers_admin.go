package api

import (
	"net/http"
	"time"

	"example.com/shiptrack/internal/clock"
	"example.com/shiptrack/internal/httpx"
	"example.com/shiptrack/internal/render"
)

// stats answers the operator's status page: the record counts, the trail length, the caps the deployment
// runs with and the tenants it serves. The token secret is not in it, and neither is a partner's endpoint:
// a status page is a page an operator screenshots.
func (a *API) stats(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r)
	if !ok {
		httpx.JSONError(w, http.StatusUnauthorized, "unauthorized", "a bearer token is required")
		return
	}
	counts := a.deps.Store.Stats()
	body := map[string]any{
		"counts":      counts,
		"trail":       a.deps.Trail.Len(),
		"trail_kinds": a.deps.Trail.Kinds(),
		"tenants":     a.deps.Config.Tenants,
		"zone":        a.deps.Config.ZoneName,
		"caps": map[string]any{
			"page_size":           a.deps.Config.Caps.PageSize,
			"max_body":            a.deps.Config.Caps.MaxBody,
			"max_batch":           a.deps.Config.Caps.MaxBatch,
			"max_grams":           a.deps.Config.Caps.MaxGrams,
			"max_lines":           a.deps.Config.Caps.MaxLines,
			"close_window_months": a.deps.Config.Caps.CloseWindowMonths,
			"max_export_cells":    a.deps.Config.Caps.MaxExportCells,
		},
		"services": len(a.deps.Book.Services()),
		"partner":  p.partner.ID,
	}
	if httpx.AcceptsJSON(r) {
		httpx.JSON(w, http.StatusOK, body)
		return
	}
	cells := make([][]string, 0, len(counts))
	for _, name := range []string{"events", "shipments", "statements", "deliveries"} {
		cells = append(cells, []string{name, render.Integer(int64(counts[name]))})
	}
	page := render.Page("Status", p.partner.ID,
		render.Table{Headers: []string{"Record", "Count"}, Rows: cells, Numeric: []bool{false, true}}.HTML()+
			render.DefinitionList([][2]string{
				{"Tenants", a.deps.Config.TenantsText()},
				{"Zone", a.deps.Config.ZoneName},
				{"Trail", render.Integer(int64(a.deps.Trail.Len()))},
				{"Services", render.Integer(int64(len(a.deps.Book.Services())))},
			}))
	httpx.HTML(w, http.StatusOK, page)
}

// auditLog lists the trail, newest first. The limit is read through IntParam, which refuses a value above
// the deployment's page size instead of clamping it.
func (a *API) auditLog(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r)
	if !ok {
		httpx.JSONError(w, http.StatusUnauthorized, "unauthorized", "a bearer token is required")
		return
	}
	limit, err := httpx.IntParam(r, "limit", a.deps.Config.Caps.PageSize, 1, a.deps.Config.Caps.PageSize)
	if err != nil {
		httpx.BadRequest(w, "the limit is not accepted")
		return
	}
	entries := a.deps.Trail.Entries(limit)
	if httpx.AcceptsJSON(r) {
		httpx.JSON(w, http.StatusOK, map[string]any{"entries": entries, "count": len(entries)})
		return
	}
	rows := make([][]string, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, []string{
			render.Integer(entry.Seq),
			entry.At.UTC().Format(time.RFC3339),
			entry.Kind,
			entry.Actor,
			entry.Detail,
		})
	}
	page := render.Page("Audit trail", p.partner.ID, render.Table{
		Headers: []string{"Seq", "At", "Kind", "Actor", "Detail"},
		Rows:    rows,
		Numeric: []bool{true, false, false, false, false},
	}.HTML())
	httpx.HTML(w, http.StatusOK, page)
}

// rates lists the rate book: the services, their bands and the tax rate the close applies. The page is
// what an operator checks when a partner disputes a statement line.
func (a *API) rates(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r)
	if !ok {
		httpx.JSONError(w, http.StatusUnauthorized, "unauthorized", "a bearer token is required")
		return
	}
	services := a.deps.Book.Services()
	lines := make([]string, 0, len(services))
	bandRows := make([][]string, 0, len(services))
	for _, name := range services {
		service, err := a.deps.Book.Service(name)
		if err != nil {
			httpx.Error(w, err)
			return
		}
		lines = append(lines, name)
		for _, band := range service.SortedBands() {
			bandRows = append(bandRows, []string{
				name,
				render.Integer(band.MaxGrams),
				band.PerKg.String(),
				render.Integer(int64(band.SurchargeBasisPoints)),
			})
		}
	}
	if httpx.AcceptsJSON(r) {
		httpx.JSON(w, http.StatusOK, map[string]any{
			"services":  lines,
			"bands":     bandRows,
			"tax_basis": a.deps.Book.TaxBasisPoints(),
			"now":       clock.FormatInstant(a.now()),
		})
		return
	}
	page := render.Page("Rate book", p.partner.ID,
		render.DefinitionList([][2]string{
			{"Tax rate", render.Integer(int64(a.deps.Book.TaxBasisPoints())) + " basis points"},
			{"Services", render.Integer(int64(len(lines)))},
			{"As of", clock.FormatInstant(a.now())},
		})+
			render.Table{
				Headers: []string{"Service", "Up to grams", "Per kg", "Surcharge bp"},
				Rows:    bandRows,
				Numeric: []bool{false, true, true, true},
			}.HTML())
	httpx.HTML(w, http.StatusOK, page)
}
