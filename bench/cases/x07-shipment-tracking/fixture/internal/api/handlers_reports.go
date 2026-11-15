package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"example.com/shiptrack/internal/clock"
	"example.com/shiptrack/internal/export"
	"example.com/shiptrack/internal/httpx"
	"example.com/shiptrack/internal/render"
	"example.com/shiptrack/internal/report"
	"example.com/shiptrack/internal/workspace"
)

// listReports renders the report catalog: the name, the selection and the parameters each report takes.
func (a *API) listReports(w http.ResponseWriter, r *http.Request) {
	catalog := report.Catalog()
	if httpx.AcceptsJSON(r) {
		out := make([]map[string]any, 0, len(catalog))
		for _, definition := range catalog {
			out = append(out, map[string]any{
				"name":        definition.Name,
				"title":       definition.Title,
				"description": definition.Description,
				"filters":     definition.FilterNames(),
				"orders":      definition.OrderNames(),
				"selection":   definition.Declaration.Describe(),
			})
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"reports": out})
		return
	}
	rows := make([][]string, 0, len(catalog))
	for _, definition := range catalog {
		rows = append(rows, []string{
			definition.Name,
			definition.Title,
			fmt.Sprint(definition.Declaration.PageSize),
			fmt.Sprint(definition.Declaration.MaxRows),
			definition.Declaration.Describe(),
		})
	}
	page := render.Page("Reports", "the catalog", render.Table{
		Headers: []string{"Name", "Title", "Page", "Max", "Selection"},
		Rows:    rows,
		Numeric: []bool{false, false, true, true, false},
	}.HTML())
	httpx.HTML(w, http.StatusOK, page)
}

// runReport runs one report for the principal's partner. The request's parameters are the selection's
// parameters, and they are filled in after the partner filter is set: a partner's report is always its
// own, and a parameter that named another partner is replaced rather than refused, because the report is
// a view of the caller's data and the caller cannot widen it.
func (a *API) runReport(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r)
	if !ok {
		httpx.JSONError(w, http.StatusUnauthorized, "unauthorized", "a bearer token is required")
		return
	}
	definition, err := report.Lookup(httpx.Path(r, "name"))
	if err != nil {
		httpx.JSONError(w, http.StatusNotFound, "not found", "no such report")
		return
	}
	values, err := scopedValues(r.URL.Query(), definition, p)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	result, err := definition.Run(a.deps.Store, values)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if httpx.AcceptsJSON(r) {
		rows := make([]map[string]string, 0, len(result.Rows))
		for _, row := range result.Rows {
			rendered := make(map[string]string, len(result.Columns))
			for _, column := range result.Columns {
				rendered[column] = result.Cell(row, column)
			}
			rows = append(rows, rendered)
		}
		httpx.JSON(w, http.StatusOK, map[string]any{
			"report":  result.Name,
			"columns": result.Columns,
			"rows":    rows,
			"count":   len(rows),
			"sql":     result.SQL,
			"args":    len(result.Args),
		})
		return
	}
	cells := make([][]string, 0, len(result.Rows))
	for _, row := range result.Rows {
		cells = append(cells, result.Cells(row))
	}
	table := render.Table{
		Caption: result.Title,
		Headers: result.Columns,
		Rows:    cells,
		Numeric: result.NumericColumns(),
	}
	body := render.Breadcrumb([]string{"reports", result.Name}) +
		render.DefinitionList([][2]string{{"Selection", result.Selection}, {"Rows", render.Count(len(cells))}}) +
		table.HTML()
	httpx.HTML(w, http.StatusOK, render.Page(result.Title, p.partner.ID, body))
}

// exportReport writes one report as CSV into the workspace and answers with the bytes. The cell budget is
// the deployment's: a report whose rows times columns is above it is refused rather than truncated, so a
// partner never downloads half a month and believes it is the month.
func (a *API) exportReport(w http.ResponseWriter, r *http.Request) {
	p, ok := principalOf(r)
	if !ok {
		httpx.JSONError(w, http.StatusUnauthorized, "unauthorized", "a bearer token is required")
		return
	}
	// The route captures the whole last segment, so the suffix is checked here: a request for a name that
	// does not end in .csv is a request for an object this route does not serve.
	raw := httpx.Path(r, "name")
	if !strings.HasSuffix(raw, ".csv") {
		httpx.JSONError(w, http.StatusNotFound, "not found", "the export is served as .csv")
		return
	}
	definition, err := report.Lookup(strings.TrimSuffix(raw, ".csv"))
	if err != nil {
		httpx.JSONError(w, http.StatusNotFound, "not found", "no such report")
		return
	}
	values, err := scopedValues(r.URL.Query(), definition, p)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	result, err := definition.Run(a.deps.Store, values)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	cells := int64(len(result.Rows)) * int64(len(result.Columns))
	if cells > int64(a.deps.Config.Caps.MaxExportCells) {
		httpx.JSONError(w, http.StatusRequestEntityTooLarge, "too large",
			fmt.Sprintf("the export is %d cells, the cap is %d", cells, a.deps.Config.Caps.MaxExportCells))
		return
	}
	rows := make([][]string, 0, len(result.Rows))
	for _, row := range result.Rows {
		rows = append(rows, result.Cells(row))
	}
	var buffer bytes.Buffer
	if err := export.CSV(&buffer, result.Name, result.Selection, result.Columns, rows, result.NumericColumns()); err != nil {
		httpx.Error(w, err)
		return
	}
	period, err := exportPeriod(values, definition, a.now())
	if err != nil {
		httpx.Error(w, err)
		return
	}
	name, err := workspace.ExportName(p.partner.ID, definition.Name, period)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if _, err := a.deps.Workspace.Write(name, buffer.Bytes(), 0o644); err != nil {
		httpx.Error(w, err)
		return
	}
	if err := a.auditWrite(p, "report.export", fmt.Sprintf("%s %d rows %s", definition.Name, len(rows), period)); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.File(w, "text/csv; charset=utf-8", definition.Name+"-"+period.String()+".csv", buffer.Bytes())
}

// scopedValues fills a request's parameters in for one partner. The partner filter is set to the
// principal's partner, and an order or a limit the definition does not declare is refused by the binder.
func scopedValues(values url.Values, definition report.Definition, p principal) (url.Values, error) {
	scoped := url.Values{}
	for key, list := range values {
		scoped[key] = append([]string(nil), list...)
	}
	if _, ok := definition.Declaration.Filter("partner"); ok {
		scoped.Set("partner", p.partner.ID)
	}
	if definition.Declaration.Table != "events" {
		// Every table but events carries a tenant column as well; the partner filter already scopes the
		// selection, and a tenant parameter a request sends is replaced with the token's when the report
		// declares one.
		if _, ok := definition.Declaration.Filter("tenant"); ok {
			scoped.Set("tenant", p.partner.Tenant)
		}
	}
	if err := definition.Declaration.Validate(); err != nil {
		return nil, fmt.Errorf("api: report %q: %w", definition.Name, err)
	}
	return scoped, nil
}

// exportPeriod is the period an export is named for: the request's `period` when the report declares one,
// and otherwise the period the current instant falls in.
func exportPeriod(values url.Values, definition report.Definition, now time.Time) (clock.Period, error) {
	if _, ok := definition.Declaration.Filter("period"); ok {
		if raw := values.Get("period"); raw != "" {
			return clock.ParsePeriod(raw)
		}
	}
	return clock.PeriodOf(now), nil
}
