package httpapi

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"example.com/artifacts/internal/artifacts"
	"example.com/artifacts/internal/audit"
	"example.com/artifacts/internal/auth"
	"example.com/artifacts/internal/clock"
	"example.com/artifacts/internal/httpx"
	"example.com/artifacts/internal/ids"
	"example.com/artifacts/internal/packaging"
	"example.com/artifacts/internal/storage"
)

// artifactView is one artifact as a response carries it.
type artifactView struct {
	ID      string `json:"id"`
	Project string `json:"project"`
	Job     string `json:"job"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Digest  string `json:"digest"`
	Bytes   int64  `json:"bytes"`
	Status  string `json:"status"`
	Archive string `json:"archive,omitempty"`
	Created string `json:"created"`
	Updated string `json:"updated"`
}

// newArtifactView renders an artifact record.
func newArtifactView(record artifacts.Record) artifactView {
	return artifactView{
		ID: record.ID, Project: record.Project, Job: record.Job, Name: record.Name,
		Kind: record.Kind, Digest: record.Digest, Bytes: record.Bytes,
		Status: string(record.Status), Archive: record.Archive,
		Created: clock.Format(record.Created), Updated: clock.Format(record.Updated),
	}
}

// uploadRequest is the body of an upload.
type uploadRequest struct {
	Name string `json:"name"`
	// Kind is the artifact's media kind, such as "report" or "bundle".
	Kind string `json:"kind"`
	// Body is the artifact's content, base64 is not needed: a report is text and a bundle is small.
	Body string `json:"body"`
}

// listArtifacts answers `GET /api/artifacts`.
func (a *API) listArtifacts(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	project, err := projectOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	status := r.URL.Query().Get("status")
	if status != "" && !artifacts.Status(status).Valid() {
		httpx.FailError(w, httpx.BadRequest(fmt.Sprintf("status %q is not one of staged, packed, published, expired", status)))
		return
	}
	records := a.store.List(project)
	views := make([]artifactView, 0, len(records))
	for _, record := range records {
		if status != "" && string(record.Status) != status {
			continue
		}
		views = append(views, newArtifactView(record))
	}
	httpx.Write(w, http.StatusOK, map[string]any{
		"project": project, "artifacts": views, "count": len(views), "limit": a.cfg.PageSize,
	})
}

// getArtifact answers `GET /api/artifacts/{id}`.
func (a *API) getArtifact(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	project, err := projectOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	record, err := a.store.InProject(project, httpx.Param(r, "id"))
	if err != nil {
		httpx.FailError(w, httpx.NotFoundError("no such artifact"))
		return
	}
	httpx.Write(w, http.StatusOK, newArtifactView(record))
}

// uploadArtifact answers `POST /api/artifacts`: the body is staged in the project's tree and the record
// names its digest.
func (a *API) uploadArtifact(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	project, err := projectOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	var request uploadRequest
	if err := httpx.Decode(r, a.cfg.MaxBody, &request); err != nil {
		a.record(principal, "artifacts.write", "", audit.OutcomeRefused, err.Error())
		httpx.FailError(w, err)
		return
	}
	if !artifacts.ValidName(request.Name) {
		a.record(principal, "artifacts.write", request.Name, audit.OutcomeRefused, "the name is not an artifact name")
		httpx.FailError(w, httpx.BadRequest("the name is not an artifact name"))
		return
	}
	if request.Kind == "" {
		request.Kind = "report"
	}
	body := []byte(request.Body)
	if int64(len(body)) > a.cfg.MaxArtifact {
		a.record(principal, "artifacts.write", request.Name, audit.OutcomeRefused, "the artifact is above the cap")
		httpx.FailError(w, httpx.Unprocessable("the artifact is above the deployment cap"))
		return
	}
	now := a.nowUTC()
	job := artifacts.Job{
		ID: ids.New(ids.Job), Project: project, Kind: "upload", Status: artifacts.JobRunning,
		Attempts: 1, Created: now, Updated: now,
	}
	if err := a.store.PutJob(job); err != nil {
		a.record(principal, "artifacts.write", job.ID, audit.OutcomeFailed, err.Error())
		httpx.FailError(w, err)
		return
	}
	record, err := a.stage(project, job, request.Name, request.Kind, body)
	if err != nil {
		job.Status = artifacts.JobFailed
		job.Error = err.Error()
		job.Updated = a.nowUTC()
		_ = a.store.UpdateJob(job)
		a.record(principal, "artifacts.write", request.Name, audit.OutcomeFailed, err.Error())
		httpx.FailError(w, httpx.Unprocessable(err.Error()))
		return
	}
	job.Artifacts = []string{record.ID}
	job.Status = artifacts.JobSucceeded
	job.Updated = a.nowUTC()
	job.Finished = job.Updated
	if err := a.store.UpdateJob(job); err != nil {
		httpx.FailError(w, err)
		return
	}
	a.record(principal, "artifacts.write", record.ID, audit.OutcomeAccepted, "staged "+record.Name)
	httpx.Write(w, http.StatusCreated, map[string]any{
		"artifact": newArtifactView(record), "job": newJobView(job),
	})
}

// stage writes an artifact body into the project's staging tree and records it.
func (a *API) stage(project string, job artifacts.Job, name, kind string, body []byte) (artifacts.Record, error) {
	blob, err := storage.PutBlob(a.layout.Root(), body)
	if err != nil {
		return artifacts.Record{}, err
	}
	staging, err := a.layout.StagingDir(job.ID)
	if err != nil {
		return artifacts.Record{}, err
	}
	if _, err := storage.Write(staging, name, body, 0o644); err != nil {
		return artifacts.Record{}, err
	}
	now := a.nowUTC()
	record := artifacts.Record{
		ID: ids.New(ids.Artifact), Project: project, Job: job.ID, Name: name, Kind: kind,
		Digest: blob.Digest, Bytes: blob.Bytes, Status: artifacts.Staged,
		Created: now, Updated: now,
	}
	if err := a.store.Put(record); err != nil {
		return artifacts.Record{}, err
	}
	return record, nil
}

// packArtifact answers `POST /api/artifacts/{id}/pack` with the kind the caller asks for.
func (a *API) packArtifact(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "bundle"
	}
	a.pack(w, r, principal, kind)
}

// repackageArtifact answers `POST /api/artifacts/{id}/repackage`, which rebuilds an archive through the
// packaging tool.
func (a *API) repackageArtifact(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	a.pack(w, r, principal, "repackage")
}

// pack builds the archive of one artifact and records it.
func (a *API) pack(w http.ResponseWriter, r *http.Request, principal auth.Principal, kind string) {
	project, err := projectOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	target := httpx.Param(r, "id")
	record, err := a.store.InProject(project, target)
	if err != nil {
		a.record(principal, "artifacts.write", target, audit.OutcomeRefused, err.Error())
		httpx.FailError(w, httpx.NotFoundError("no such artifact"))
		return
	}
	staging, err := a.layout.StagingDir(record.Job)
	if err != nil {
		a.record(principal, "artifacts.write", target, audit.OutcomeFailed, err.Error())
		httpx.FailError(w, err)
		return
	}
	packs, err := a.layout.PackDir(record.Job)
	if err != nil {
		a.record(principal, "artifacts.write", target, audit.OutcomeFailed, err.Error())
		httpx.FailError(w, err)
		return
	}
	result, err := a.packer.Pack(r.Context(), packaging.Request{
		Kind: kind, SourceDir: staging, OutputDir: packs, Name: record.Name,
	})
	if err != nil {
		a.record(principal, "artifacts.write", target, audit.OutcomeRefused, err.Error())
		httpx.FailError(w, httpx.Unprocessable(err.Error()))
		return
	}
	record.Archive = filepath.Base(result.Path)
	record.Status = artifacts.Packed
	record.Updated = a.nowUTC()
	if err := a.store.Update(record); err != nil {
		a.record(principal, "artifacts.write", target, audit.OutcomeFailed, err.Error())
		httpx.FailError(w, err)
		return
	}
	a.record(principal, "artifacts.write", target, audit.OutcomeAccepted, "packed "+kind)
	httpx.Write(w, http.StatusOK, map[string]any{
		"artifact": newArtifactView(record),
		"pack": map[string]any{
			"kind": result.Kind, "path": filepath.Base(result.Path),
			"bytes": result.Bytes, "entries": result.Entries, "tool": result.Tool,
		},
	})
}

// deleteArtifact answers `DELETE /api/artifacts/{id}`. The record is removed and the blob is dropped
// only when no other artifact names the same digest.
func (a *API) deleteArtifact(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	project, err := projectOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	target := httpx.Param(r, "id")
	record, err := a.store.InProject(project, target)
	if err != nil {
		a.record(principal, "artifacts.write", target, audit.OutcomeRefused, err.Error())
		httpx.FailError(w, httpx.NotFoundError("no such artifact"))
		return
	}
	if _, err := a.store.Delete(record.ID); err != nil {
		a.record(principal, "artifacts.write", target, audit.OutcomeFailed, err.Error())
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
	a.record(principal, "artifacts.write", target, audit.OutcomeAccepted, "deleted "+record.Name)
	httpx.Write(w, http.StatusOK, map[string]any{"deleted": newArtifactView(record)})
}

// downloadArtifact answers `GET /api/artifacts/{id}/content` with the artifact's bytes, read through the
// tree's own reader.
func (a *API) downloadArtifact(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	project, err := projectOf(principal)
	if err != nil {
		httpx.FailError(w, err)
		return
	}
	record, err := a.store.InProject(project, httpx.Param(r, "id"))
	if err != nil {
		httpx.FailError(w, httpx.NotFoundError("no such artifact"))
		return
	}
	body, err := storage.ReadBlob(a.layout.Root(), record.Digest)
	if err != nil {
		httpx.FailError(w, httpx.NotFoundError("the artifact's content is not stored"))
		return
	}
	contentType := "application/octet-stream"
	if strings.HasSuffix(record.Name, ".csv") || record.Kind == "report" {
		contentType = "text/csv"
	}
	httpx.Attachment(w, contentType, record.Name, string(body))
}
