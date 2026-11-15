package httpapi

import (
	"fmt"
	"net/http"

	"example.com/artifacts/internal/artifacts"
	"example.com/artifacts/internal/auth"
	"example.com/artifacts/internal/clock"
	"example.com/artifacts/internal/httpx"
)

// jobView is one job as a response carries it.
type jobView struct {
	ID        string   `json:"id"`
	Project   string   `json:"project"`
	Kind      string   `json:"kind"`
	Status    string   `json:"status"`
	Artifacts []string `json:"artifacts"`
	Attempts  int      `json:"attempts"`
	Error     string   `json:"error,omitempty"`
	Created   string   `json:"created"`
	Updated   string   `json:"updated"`
}

// newJobView renders a job.
func newJobView(job artifacts.Job) jobView {
	return jobView{
		ID: job.ID, Project: job.Project, Kind: job.Kind, Status: string(job.Status),
		Artifacts: job.Artifacts, Attempts: job.Attempts, Error: job.Error,
		Created: clock.Format(job.Created), Updated: clock.Format(job.Updated),
	}
}

// listJobs answers `GET /api/jobs`.
func (a *API) listJobs(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	project, err := projectOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	status := r.URL.Query().Get("status")
	if status != "" && !artifacts.JobStatus(status).Valid() {
		httpx.FailError(w, httpx.BadRequest(fmt.Sprintf("status %q is not one of queued, running, succeeded, failed", status)))
		return
	}
	jobs := a.store.Jobs(project)
	views := make([]jobView, 0, len(jobs))
	for _, job := range jobs {
		if status != "" && string(job.Status) != status {
			continue
		}
		views = append(views, newJobView(job))
	}
	httpx.Write(w, http.StatusOK, map[string]any{
		"project": project, "jobs": views, "count": len(views),
	})
}

// getJob answers `GET /api/jobs/{id}`.
func (a *API) getJob(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	project, err := projectOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	job, err := a.store.Job(httpx.Param(r, "id"))
	if err != nil || job.Project != project {
		httpx.FailError(w, httpx.NotFoundError("no such job"))
		return
	}
	records := a.store.OfJob(job.ID)
	views := make([]artifactView, 0, len(records))
	for _, record := range records {
		views = append(views, newArtifactView(record))
	}
	httpx.Write(w, http.StatusOK, map[string]any{"job": newJobView(job), "artifacts": views})
}

// jobProgress reports how many of a job's artifacts are packed, which the console prints.
func jobProgress(job artifacts.Job, records []artifacts.Record) int {
	packed := 0
	for _, record := range records {
		if record.Status == artifacts.Packed || record.Status == artifacts.Published {
			packed++
		}
	}
	return packed
}

// jobsHandlerNames lists the route patterns the job handlers answer, for the console page.
func (a *API) jobsHandlerNames() []string {
	var out []string
	for _, pattern := range a.Patterns() {
		if pattern == "GET /api/jobs" || pattern == "GET /api/jobs/{id}" {
			out = append(out, pattern)
		}
	}
	return out
}
