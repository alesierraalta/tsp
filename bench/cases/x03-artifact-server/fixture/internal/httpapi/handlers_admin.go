package httpapi

import (
	"net/http"

	"example.com/artifacts/internal/artifacts"
	"example.com/artifacts/internal/audit"
	"example.com/artifacts/internal/auth"
	"example.com/artifacts/internal/clock"
	"example.com/artifacts/internal/httpx"
	"example.com/artifacts/internal/storage"
)

// console answers `GET /api/admin/console` with the configuration, the counts and the trail's summary.
func (a *API) console(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if _, err := projectOf(principal); err != nil {
		httpx.FailError(w, err)
		return
	}
	stats := a.store.Stats()
	records := make([]map[string]any, 0, 2)
	for _, name := range []string{"artifacts", "jobs"} {
		records = append(records, map[string]any{"name": name, "count": stats[name]})
	}
	httpx.Write(w, http.StatusOK, map[string]any{
		"config":    a.cfg.String(),
		"operator":  principal.Subject,
		"records":   records,
		"bytes":     a.store.Bytes(),
		"trail":     a.trail.Summary(),
		"dropped":   a.trail.Dropped(),
		"routes":    len(a.Patterns()),
		"allowlist": a.relay.Guard().Allowlist().Entries(),
		"at":        clock.Format(a.nowUTC()),
	})
}

// auditTrail answers `GET /api/admin/audit` with the trail of the caller's project.
func (a *API) auditTrail(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	project, err := projectOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	records := a.trail.OfProject(project)
	views := make([]map[string]any, 0, len(records))
	for _, record := range records {
		views = append(views, map[string]any{
			"id": record.ID, "at": clock.Format(record.At), "subject": record.Subject,
			"action": record.Action, "target": record.Target,
			"outcome": record.Outcome, "detail": record.Detail,
		})
	}
	httpx.Write(w, http.StatusOK, map[string]any{
		"project": project, "records": views, "count": len(views),
		"summary": a.trail.Summary(), "dropped": a.trail.Dropped(),
	})
}

// sweepResponse is what a sweep answers with.
type sweepResponse struct {
	Expired   int      `json:"expired"`
	Artifacts []string `json:"artifacts"`
	Bytes     int64    `json:"bytes"`
	At        string   `json:"at"`
}

// sweep answers `POST /api/admin/sweep`: the artifacts past the retention window are removed, the
// project's newest ones are kept, and the blobs nobody names any more are dropped with them.
func (a *API) sweep(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	project, err := projectOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	policy := artifacts.Policy{
		Days: a.cfg.Retention, KeepLast: a.cfg.KeepLast, KeepStaged: false,
	}
	now := a.nowUTC()
	removable, err := artifacts.Sweep(policy, a.store.List(project), now)
	if err != nil {
		a.record(principal, "admin.operate", "", audit.OutcomeFailed, err.Error())
		httpx.FailError(w, httpx.Unprocessable(err.Error()))
		return
	}
	response := sweepResponse{Artifacts: make([]string, 0, len(removable)), At: clock.Format(now)}
	for _, record := range removable {
		if _, err := a.store.Delete(record.ID); err != nil {
			a.record(principal, "admin.operate", record.ID, audit.OutcomeFailed, err.Error())
			httpx.FailError(w, err)
			return
		}
		shared := false
		for _, other := range a.store.All() {
			if other.Digest == record.Digest {
				shared = true
				break
			}
		}
		if !shared {
			_ = storage.RemoveBlob(a.layout.Root(), record.Digest)
		}
		response.Artifacts = append(response.Artifacts, record.ID)
		response.Bytes += record.Bytes
	}
	response.Expired = len(response.Artifacts)
	a.record(principal, "admin.operate", "", audit.OutcomeAccepted, "expired "+itoa(response.Expired))
	httpx.Write(w, http.StatusOK, response)
}
