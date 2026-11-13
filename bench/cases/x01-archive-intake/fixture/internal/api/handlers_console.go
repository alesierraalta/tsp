package api

import (
	"net/http"
	"strconv"

	"example.com/archive/internal/audit"
	"example.com/archive/internal/auth"
	"example.com/archive/internal/httpx"
	"example.com/archive/internal/render"
)

// handleConsole renders the operator console: the configuration, the store summary, the staged batches and
// the trail's shape. It is the page a deployment links from its own operations notes.
func (a *API) handleConsole(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.principal(w, r, auth.ActionAdmin)
	if !ok {
		return
	}
	layouts, err := a.staging.Layouts()
	if err != nil {
		a.fail(w, r, principal, auth.ActionAdmin, err)
		return
	}
	staged := make([]string, 0, len(layouts))
	for _, layout := range layouts {
		staged = append(staged, layout.Batch())
	}
	a.record(principal, auth.ActionAdmin, "/api/admin/console", audit.OutcomeOK, "console")
	page := render.Console(a.cfg, a.store.Stats(), staged, a.trail, a.clock())
	httpx.HTML(w, http.StatusOK, page)
}

// handleVerify runs the store's integrity pass and reports what it found. It is a read: the pass opens
// blobs and compares digests, and it never rewrites a file.
func (a *API) handleVerify(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.principal(w, r, auth.ActionAdmin)
	if !ok {
		return
	}
	report, err := a.store.Verify()
	if err != nil {
		a.fail(w, r, principal, auth.ActionAdmin, err)
		return
	}
	outcome := audit.OutcomeOK
	if !report.OK() {
		outcome = audit.OutcomeFailed
	}
	a.record(principal, auth.ActionAdmin, "/api/admin/verify", outcome, strconv.Itoa(report.Problems)+" problems")
	if httpx.WantsHTML(r) {
		httpx.HTML(w, http.StatusOK, render.Page("Integrity", render.LineList(report.Describe())))
		return
	}
	httpx.JSON(w, http.StatusOK, report)
}

// handleStagedListing serves what one staged batch holds, file by file. The listing is the console's answer
// to "what did the delivery actually carry", so an operator can compare it with the manifest.
func (a *API) handleStagedListing(w http.ResponseWriter, r *http.Request) {
	principal, ok := a.principal(w, r, auth.ActionBatchView)
	if !ok {
		return
	}
	layout, _, ok := a.batchManifest(w, r, principal)
	if !ok {
		return
	}
	listing, err := layout.List()
	if err != nil {
		a.fail(w, r, principal, auth.ActionBatchView, err)
		return
	}
	a.record(principal, auth.ActionBatchView, layout.Batch(), audit.OutcomeOK, strconv.Itoa(len(listing.Files))+" files")
	httpx.JSON(w, http.StatusOK, map[string]any{
		"batch": listing.Batch,
		"files": listing.Files,
		"bytes": listing.Bytes,
		"roles": listing.Roles(),
	})
}
