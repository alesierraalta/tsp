package api

import (
	"fmt"
	"net/http"
	"strings"

	"example.com/docvault/internal/audit"
	"example.com/docvault/internal/auth"
	"example.com/docvault/internal/httpx"
	"example.com/docvault/internal/report"
	"example.com/docvault/internal/vault"
)

// handleStats reports the shape of the caller's document set.
func (a *API) handleStats(w http.ResponseWriter, r *http.Request) {
	pr, ok := a.principal(w, r, auth.ActionAdmin)
	if !ok {
		return
	}
	docs := a.vault.Store().List(vault.Query{Tenant: pr.Tenant})
	counts := report.Tally(docs)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"counts":  counts,
		"kinds":   report.KindBreakdown(counts),
		"tenants": a.vault.Store().Tenants(),
		"trail":   a.trail.Len(),
		"routes":  len(a.Routes()),
	})
}

// handleReindex rebuilds the metadata index from the object tree. The tree is listed through the
// vault, so a reindex can only ever see objects inside the root.
func (a *API) handleReindex(w http.ResponseWriter, r *http.Request) {
	pr, ok := a.principal(w, r, auth.ActionAdmin)
	if !ok {
		return
	}
	prefix := strings.TrimSpace(r.URL.Query().Get("prefix"))
	if prefix == "" {
		prefix = "documents"
	}
	dry := queryBool(r, "dry")
	keys, err := a.vault.List(prefix)
	if err != nil {
		a.trail.Record(pr.Subject, pr.Tenant, auth.ActionAdmin, prefix, audit.OutcomeRefused, err.Error())
		a.fail(w, r, err)
		return
	}
	indexed := 0
	for _, key := range keys {
		if _, err := a.vault.Store().Get(key); err == nil {
			continue
		}
		if dry {
			indexed++
			continue
		}
		size, updated, err := a.vault.Stat(key)
		if err != nil {
			continue
		}
		a.vault.Store().Put(vault.Doc{
			ID:      docID(key),
			Key:     key,
			Tenant:  pr.Tenant,
			Title:   key,
			Kind:    "article",
			Owner:   pr.Subject,
			Size:    size,
			Updated: updated,
		})
		indexed++
	}
	a.trail.Record(pr.Subject, pr.Tenant, auth.ActionAdmin, prefix, audit.OutcomeOK,
		fmt.Sprintf("indexed %d objects (dry=%v)", indexed, dry))
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"scanned": len(keys), "indexed": indexed, "dry": dry})
}

// handlePurge empties the audit trail of the caller's tenant.
func (a *API) handlePurge(w http.ResponseWriter, r *http.Request) {
	pr, ok := a.principal(w, r, auth.ActionAdmin)
	if !ok {
		return
	}
	before := a.trail.Len()
	a.trail.Reset(pr.Subject, pr.Tenant)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"dropped": before})
}

// handleAudit returns the audit entries of the caller's tenant.
func (a *API) handleAudit(w http.ResponseWriter, r *http.Request) {
	pr, ok := a.principal(w, r, auth.ActionAuditView)
	if !ok {
		return
	}
	limit := a.limits.ClampPage(queryInt(r, "limit", 0))
	entries := a.trail.OfTenant(pr.Tenant, limit)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entries": entries, "limit": limit})
}

// handleExport returns the caller's document set as CSV.
func (a *API) handleExport(w http.ResponseWriter, r *http.Request) {
	pr, ok := a.principal(w, r, auth.ActionExport)
	if !ok {
		return
	}
	docs := a.vault.Store().List(vault.Query{
		Tenant: pr.Tenant,
		Kind:   strings.TrimSpace(r.URL.Query().Get("kind")),
	})
	body, err := report.CSV(docs)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.trail.Record(pr.Subject, pr.Tenant, auth.ActionExport, "", audit.OutcomeOK,
		fmt.Sprintf("exported %d documents", len(docs)))
	w.Header().Set("Content-Disposition", "attachment; filename=\"documents.csv\"")
	httpx.WriteText(w, http.StatusOK, body)
}
