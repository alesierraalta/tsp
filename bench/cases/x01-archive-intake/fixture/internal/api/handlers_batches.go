package api

import (
	"example.com/archive/internal/audit"
	"example.com/archive/internal/auth"
	"example.com/archive/internal/checksum"
	"example.com/archive/internal/httpx"
	"example.com/archive/internal/intake"
	"example.com/archive/internal/render"
	"fmt"
	"net/http"
)

// handleBatchCreate accepts one multipart delivery, stages it, runs the pipeline, and answers with the
// result. The tenant and the operator come from the token, never from the manifest: a manifest that
// names another tenant is overwritten, and the audit trail names the subject that delivered it.
func (a *API) handleBatchCreate(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.principal(w, r, auth.ActionBatchRun)
	if !ok {
		return
	}
	delivery, err := httpx.ReadDelivery(r, httpx.Limits{
		MaxBodyBytes: a.lim.MaxBodyBytes,
		MaxUpload:    a.lim.MaxUpload,
		MaxParts:     a.lim.MaxEntries*3 + 16,
	})
	if err != nil {
		a.badRequest(w, r, principal, auth.ActionBatchRun, err)
		return
	}
	m := delivery.Manifest
	m.Tenant = principal.Tenant
	if m.Operator == "" {
		m.Operator = principal.Subject
	}
	if m.Operator != principal.Subject {
		a.refuse(w, r, principal, auth.ActionBatchRun, "manifest operator is not the token's subject")
		return
	}
	result, err := a.runner.Accept(r.Context(), m, intake.Upload{
		Containers: delivery.Upload.Containers,
		Pages:      delivery.Upload.Pages,
		Sidecars:   delivery.Upload.Sidecars,
	})
	if err != nil {
		a.fail(w, r, principal, auth.ActionBatchRun, err)
		return
	}
	a.reindex(result.Batch)
	outcome := audit.OutcomeOK
	detail := fmt.Sprintf("%d entries, %d pages", len(result.Entries), result.Pages)
	if result.Failed() > 0 {
		outcome = audit.OutcomeFailed
		detail = fmt.Sprintf("%d entries failed", result.Failed())
	}
	a.record(principal, auth.ActionBatchRun, result.Batch, outcome, detail)
	httpx.JSON(w, http.StatusCreated, resultView(result))
}

// handleBatchList lists the batches the principal's tenant has staged, most recent first.
func (a *API) handleBatchList(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.principal(w, r, auth.ActionBatchView)
	if !ok {
		return
	}
	layouts, err := a.staging.Layouts()
	if err != nil {
		a.fail(w, r, principal, auth.ActionBatchView, err)
		return
	}
	states := make([]map[string]any, 0, len(layouts))
	for _, layout := range layouts {
		state, err := a.runner.State(layout.Batch())
		if err != nil {
			continue
		}
		if state.Tenant != principal.Tenant {
			continue
		}
		states = append(states, stateView(state))
	}
	a.record(principal, auth.ActionBatchView, "/api/batches", audit.OutcomeOK, fmt.Sprintf("%d batches", len(states)))
	httpx.JSON(w, http.StatusOK, map[string]any{"batches": states})
}

// handleBatch serves one staged batch's state, as JSON or as the console's page.
func (a *API) handleBatch(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.principal(w, r, auth.ActionBatchView)
	if !ok {
		return
	}
	state, ok := a.batchState(w, r, principal)
	if !ok {
		return
	}
	a.record(principal, auth.ActionBatchView, state.Batch, audit.OutcomeOK, "state")
	if httpx.WantsHTML(r) {
		httpx.HTML(w, http.StatusOK, render.ResumeReport(state, a.clock()))
		return
	}
	httpx.JSON(w, http.StatusOK, stateView(state))
}

// handleBatchResume continues an interrupted batch. The batch's tenant is checked before the pipeline
// is asked to do anything, so a token cannot resume another tenant's delivery.
func (a *API) handleBatchResume(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.principal(w, r, auth.ActionBatchRun)
	if !ok {
		return
	}
	state, ok := a.batchState(w, r, principal)
	if !ok {
		return
	}
	result, err := a.runner.Resume(r.Context(), state.Batch)
	if err != nil {
		a.fail(w, r, principal, auth.ActionBatchRun, err)
		return
	}
	a.reindex(result.Batch)
	outcome := audit.OutcomeOK
	if result.Failed() > 0 {
		outcome = audit.OutcomeFailed
	}
	a.record(principal, auth.ActionBatchRun, result.Batch, outcome, "resumed: "+result.Summary())
	httpx.JSON(w, http.StatusOK, resultView(result))
}

// handleBatchAbort discards a staged batch's working tree. A batch that already committed is refused:
// its objects are the archive's, and deleting them is the object endpoint's job.
func (a *API) handleBatchAbort(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.principal(w, r, auth.ActionBatchRun)
	if !ok {
		return
	}
	state, ok := a.batchState(w, r, principal)
	if !ok {
		return
	}
	if err := a.runner.Abort(state.Batch); err != nil {
		a.fail(w, r, principal, auth.ActionBatchRun, err)
		return
	}
	a.record(principal, auth.ActionBatchRun, state.Batch, audit.OutcomeOK, "aborted")
	w.WriteHeader(http.StatusNoContent)
}

// handleBatchReport renders the import report of one batch: what the manifest declared, what the
// pipeline did, and the provenance of every entry. It is the page an operator keeps with a delivery.
func (a *API) handleBatchReport(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.principal(w, r, auth.ActionBatchView)
	if !ok {
		return
	}
	layout, m, ok := a.batchManifest(w, r, principal)
	if !ok {
		return
	}
	records := a.runner.RecordsOf(layout.Batch())
	result := intake.Result{Batch: layout.Batch(), Tenant: m.Tenant, Committed: true}
	for _, record := range records {
		result.Entries = append(result.Entries, intake.EntryResult{
			Name:   record.Entry,
			Object: record.ID,
			Kind:   record.Kind,
			Pages:  record.Pages,
			Bytes:  record.Bytes,
			Digest: checksum.Short(record.Digest),
		})
		result.Pages += record.Pages
		result.Bytes += record.Bytes
	}
	a.record(principal, auth.ActionBatchView, layout.Batch(), audit.OutcomeOK, "report")
	httpx.HTML(w, http.StatusOK, render.BatchReport(m, result, records, a.clock()))
}
