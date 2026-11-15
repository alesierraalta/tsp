package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"example.com/archive/internal/audit"
	"example.com/archive/internal/auth"
	"example.com/archive/internal/clock"
	"example.com/archive/internal/export"
	"example.com/archive/internal/httpx"
	"example.com/archive/internal/render"
	"example.com/archive/internal/retention"
)

// handleHealth reports that the process is up. It reads nothing from the store: a health check that
// reads the store turns a slow store into a restart loop.
func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"now":    clock.Stamp(a.clock()),
	})
}

// handleStats is the operator's summary: objects, bytes, pages, the staged batches and the trail. It
// needs the admin scope, because the numbers describe every tenant the deployment serves.
func (a *API) handleStats(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.principal(w, r, auth.ActionAdmin)
	if !ok {
		return
	}
	stats := a.store.Stats()
	used, err := a.staging.Used()
	if err != nil {
		a.fail(w, r, principal, auth.ActionAdmin, err)
		return
	}
	layouts, err := a.staging.Layouts()
	if err != nil {
		a.fail(w, r, principal, auth.ActionAdmin, err)
		return
	}
	view := map[string]any{
		"objects":        stats.Objects,
		"bytes":          stats.Bytes,
		"pages":          stats.Pages,
		"blobs":          stats.Blobs,
		"kinds":          stats.Kinds,
		"tenants":        stats.Tenants,
		"indexed":        a.index.Len(),
		"staged_batches": len(layouts),
		"staged_bytes":   used,
		"trail":          a.trail.Summarize(),
		"retention":      a.retentionPolicy().Describe(),
		"zone":           a.cfg.Zone,
		"now":            clock.Stamp(a.clock()),
	}
	a.record(principal, auth.ActionAdmin, "/api/admin/stats", audit.OutcomeOK, "stats")
	if httpx.WantsHTML(r) {
		httpx.HTML(w, http.StatusOK, render.Page("Admin", render.DefinitionList([][2]string{
			{"objects", strconv.Itoa(stats.Objects)},
			{"bytes", strconv.FormatInt(stats.Bytes, 10)},
			{"pages", strconv.Itoa(stats.Pages)},
			{"blobs", strconv.Itoa(stats.Blobs)},
			{"staged batches", strconv.Itoa(len(layouts))},
			{"staged bytes", strconv.FormatInt(used, 10)},
			{"trail", a.trail.Summarize()},
			{"retention", a.retentionPolicy().Describe()},
		})))
		return
	}
	httpx.JSON(w, http.StatusOK, view)
}

// handleAudit serves the trail. The query filter is read once, and a subject that asks for another
// tenant's entries is answered with its own: the tenant is the token's, always.
func (a *API) handleAudit(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.principal(w, r, auth.ActionAudit)
	if !ok {
		return
	}
	limit, err := httpx.IntQuery(r, "limit", a.cfg.PageSize, a.lim.MaxExportRows)
	if err != nil {
		a.refuse(w, r, principal, auth.ActionAudit, err.Error())
		return
	}
	filter := audit.Filter{
		Tenant:  principal.Tenant,
		Subject: httpx.Query(r, "subject"),
		Action:  httpx.Query(r, "action"),
		Outcome: httpx.Query(r, "outcome"),
		Query:   httpx.Query(r, "q"),
		Limit:   limit,
	}
	entries := a.trail.Select(filter)
	a.record(principal, auth.ActionAudit, "/api/admin/audit", audit.OutcomeOK, strconv.Itoa(len(entries))+" rows")
	if httpx.WantsHTML(r) {
		rows := make([][]string, 0, len(entries))
		for _, entry := range entries {
			rows = append(rows, []string{entry.At, entry.Subject, entry.Tenant, entry.Action, entry.Target, entry.Outcome, entry.Detail})
		}
		page := render.Page("Audit", render.Table([]string{"at", "subject", "tenant", "action", "target", "outcome", "detail"}, rows))
		httpx.HTML(w, http.StatusOK, page)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"entries": entries, "total": a.trail.Len()})
}

// handleSweep removes the closed batches past the retention window and the staged trees past the
// staging window. It is the one endpoint that deletes more than one batch, so it needs the admin scope.
func (a *API) handleSweep(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.principal(w, r, auth.ActionAdmin)
	if !ok {
		return
	}
	now := a.clock()
	policy := a.retentionPolicy()
	candidates := make([]retention.Candidate, 0, 16)
	closed := map[string]bool{}
	for _, record := range a.store.All() {
		if closed[record.Batch] {
			continue
		}
		closed[record.Batch] = true
		candidates = append(candidates, retention.Candidate{ID: record.Batch, Closed: record.CommittedAt()})
	}
	verdicts := retention.Sweep(candidates, policy, now)
	removed := 0
	failed := 0
	for _, verdict := range verdicts {
		if !verdict.Remove {
			continue
		}
		layout, err := a.staging.Layout(verdict.ID)
		if err == nil && layout.Exists() {
			if err := layout.Remove(); err != nil {
				failed++
				continue
			}
		}
		removed++
	}
	staged, err := a.staging.Sweep(24 * time.Hour)
	if err != nil {
		a.fail(w, r, principal, auth.ActionAdmin, err)
		return
	}
	a.record(principal, auth.ActionAdmin, "/api/admin/sweep", audit.OutcomeOK,
		fmt.Sprintf("%d batches past the window, %d staged trees, %d failures", removed, len(staged), failed))
	httpx.JSON(w, http.StatusOK, map[string]any{
		"removed":        removed,
		"staged_removed": staged,
		"failed":         failed,
		"policy":         policy.Describe(),
		"verdicts":       verdicts,
	})
}

// retentionPolicy is the policy the configured window and the batch cap mean.
func (a *API) retentionPolicy() retention.Policy {
	return retention.Policy{KeepDays: a.cfg.RetentionDays, KeepBatches: 3, KeepPinned: true}
}

// exportName is the file name the console's export link carries.
func (a *API) exportName(tenant string) string {
	return export.Filename(tenant, a.clock())
}
