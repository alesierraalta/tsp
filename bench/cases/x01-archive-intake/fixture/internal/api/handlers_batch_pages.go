package api

import (
	"example.com/archive/internal/audit"
	"example.com/archive/internal/auth"
	"example.com/archive/internal/httpx"
	"example.com/archive/internal/intake"
	"example.com/archive/internal/manifest"
	"example.com/archive/internal/staging"
	"fmt"
	"net/http"
)

// handleBatchPage serves one page image of a staged scan. The page index names the file, so the read
// goes through the layout's page reader, which refuses a name that does not sit in the entry's own
// directory.
func (a *API) handleBatchPage(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.principal(w, r, auth.ActionBatchView)
	if !ok {
		return
	}
	layout, m, ok := a.batchManifest(w, r, principal)
	if !ok {
		return
	}
	entryName := httpx.Query(r, "entry")
	if entryName == "" {
		a.refuse(w, r, principal, auth.ActionBatchView, "entry is required")
		return
	}
	entry, found := m.EntryByName(entryName)
	if !found {
		a.notFound(w, r, principal, auth.ActionBatchView, "no such entry")
		return
	}
	number, err := paramInt(r, "page")
	if err != nil {
		a.badRequest(w, r, principal, auth.ActionBatchView, err)
		return
	}
	if number > a.pageCap() {
		a.refuse(w, r, principal, auth.ActionBatchView, fmt.Sprintf("page %d is beyond the %d this deployment stages", number, a.pageCap()))
		return
	}
	pages, err := layout.ReadPageIndex(entry)
	if err != nil {
		a.fail(w, r, principal, auth.ActionBatchView, err)
		return
	}
	var page staging.Page
	for _, candidate := range pages {
		if candidate.Number == number {
			page = candidate
			break
		}
	}
	if page.Number == 0 {
		a.notFound(w, r, principal, auth.ActionBatchView, "no such page")
		return
	}
	body, err := layout.OpenPage(entry, page)
	if err != nil {
		a.fail(w, r, principal, auth.ActionBatchView, err)
		return
	}
	a.record(principal, auth.ActionBatchView, entry.Name, audit.OutcomeOK, fmt.Sprintf("page %d, %d bytes", page.Number, len(body)))
	httpx.Bytes(w, http.StatusOK, "image/x-portable-graymap", body)
}

// batchState reads the staged batch the route names and proves its tenant is the principal's.
func (a *API) batchState(w http.ResponseWriter, r *http.Request, p auth.Principal) (intake.ResumeState, bool) {
	id := httpx.Param(r, "id")
	state, err := a.runner.State(id)
	if err != nil {
		a.notFound(w, r, p, auth.ActionBatchView, "no such batch")
		return intake.ResumeState{}, false
	}
	if state.Tenant != p.Tenant {
		a.notFound(w, r, p, auth.ActionBatchView, "no such batch")
		return intake.ResumeState{}, false
	}
	return state, true
}

// batchManifest reads the staged batch's manifest and proves its tenant is the principal's.
func (a *API) batchManifest(w http.ResponseWriter, r *http.Request, p auth.Principal) (staging.Layout, manifest.Manifest, bool) {
	id := httpx.Param(r, "id")
	layout, err := a.staging.Layout(id)
	if err != nil || !layout.Exists() {
		a.notFound(w, r, p, auth.ActionBatchView, "no such batch")
		return staging.Layout{}, manifest.Manifest{}, false
	}
	m, err := layout.ReadManifest()
	if err != nil {
		a.fail(w, r, p, auth.ActionBatchView, err)
		return staging.Layout{}, manifest.Manifest{}, false
	}
	if m.Tenant != p.Tenant {
		a.notFound(w, r, p, auth.ActionBatchView, "no such batch")
		return staging.Layout{}, manifest.Manifest{}, false
	}
	return layout, m, true
}

// reindex indexes the objects one batch committed, so a search sees them without a restart.
func (a *API) reindex(batch string) {
	for _, record := range a.runner.RecordsOf(batch) {
		text, err := a.store.ReadText(record.ID)
		if err != nil {
			text = ""
		}
		a.index.Add(record, text)
	}
}

// resultView is the JSON shape one pipeline result is served as.
func resultView(result intake.Result) map[string]any {
	view := map[string]any{
		"batch":     result.Batch,
		"tenant":    result.Tenant,
		"entries":   result.Entries,
		"pages":     result.Pages,
		"bytes":     result.Bytes,
		"committed": result.Committed,
		"summary":   result.Summary(),
	}
	if result.Failed() > 0 {
		view["failed"] = result.Failed()
	}
	return view
}

// stateView is the JSON shape one staged batch's state is served as.
func stateView(state intake.ResumeState) map[string]any {
	entries := make([]map[string]any, 0, len(state.Entries))
	for _, entry := range state.Entries {
		entries = append(entries, map[string]any{
			"name":      entry.Name,
			"kind":      entry.Kind,
			"pages":     entry.Pages,
			"staged":    entry.Staged,
			"committed": entry.Committed,
			"object":    entry.Object,
		})
	}
	return map[string]any{
		"batch":     state.Batch,
		"tenant":    state.Tenant,
		"entries":   entries,
		"committed": state.Committed,
		"used":      state.Used,
	}
}
