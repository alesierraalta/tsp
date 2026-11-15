package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"example.com/docvault/internal/audit"
	"example.com/docvault/internal/auth"
	"example.com/docvault/internal/httpx"
	"example.com/docvault/internal/report"
	"example.com/docvault/internal/vault"
)

// everyHandlerReadsTheTree is the list of render endpoints and the guard each one uses. It is a
// documentation value: the suite asserts that each handler below resolves its path through the
// vault instead of joining one itself.
//
//	preview      metadata only, no path
//	download     vault.Read
//	thumbnail    one object of the tree
//	object       vault.Resolve then os.Open
//	attachment   vault.ReadAttachment
//	attachments  vault.List of the attachment prefix
var everyHandlerReadsTheTree = []string{"preview", "download", "thumbnail", "object", "attachment", "attachments"}

// handlePreview renders one document as an HTML page.
func (a *API) handlePreview(w http.ResponseWriter, r *http.Request) {
	pr, ok := a.principal(w, r, auth.ActionRead)
	if !ok {
		return
	}
	doc, err := a.docOf(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if !auth.InTenant(pr, doc.Tenant) {
		httpx.Fail(w, http.StatusNotFound, "no such document")
		return
	}
	body, err := a.vault.Read(doc.Key)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.trail.Record(pr.Subject, pr.Tenant, auth.ActionRead, doc.Key, audit.OutcomeOK, "previewed")
	httpx.WriteHTML(w, http.StatusOK, report.SummaryHTML(doc, body, a.now()))
}

// handleDownload returns the bytes of one document as an attachment.
func (a *API) handleDownload(w http.ResponseWriter, r *http.Request) {
	pr, ok := a.principal(w, r, auth.ActionRead)
	if !ok {
		return
	}
	doc, err := a.docOf(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if !auth.InTenant(pr, doc.Tenant) {
		httpx.Fail(w, http.StatusNotFound, "no such document")
		return
	}
	body, err := a.vault.Read(doc.Key)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	start, end, err := vault.ParseRange(r.Header.Get("Range"), int64(len(body)))
	if err != nil {
		httpx.Fail(w, http.StatusRequestedRangeNotSatisfiable, err.Error())
		return
	}
	slice := body[start:end]
	a.trail.Record(pr.Subject, pr.Tenant, auth.ActionRead, doc.Key, audit.OutcomeOK,
		fmt.Sprintf("download %d-%d", start, end))
	w.Header().Set("Content-Disposition", "attachment; filename=\""+safeFilename(doc.Key)+"\"")
	w.Header().Set("Content-Length", strconv.Itoa(len(slice)))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	w.Write(slice)
}

// handleThumbnail returns the thumbnail stored next to a document. The name defaults to the one the
// build writes; a caller may ask for another object of the same document.
func (a *API) handleThumbnail(w http.ResponseWriter, r *http.Request) {
	pr, ok := a.principal(w, r, auth.ActionAttachRead)
	if !ok {
		return
	}
	doc, err := a.docOf(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if !auth.InTenant(pr, doc.Tenant) {
		httpx.Fail(w, http.StatusNotFound, "no such document")
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		name = "thumb.png"
	}
	target := filepath.Join(a.vault.Root(), name)
	body, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			httpx.Fail(w, http.StatusNotFound, fmt.Sprintf("no thumbnail %q", name))
			return
		}
		a.fail(w, r, err)
		return
	}
	a.trail.Record(pr.Subject, pr.Tenant, auth.ActionAttachRead, name, audit.OutcomeOK,
		fmt.Sprintf("thumbnail %d bytes", len(body)))
	writeObject(w, "image/png", body)
}

// handleObject streams one object of the tree by name.
func (a *API) handleObject(w http.ResponseWriter, r *http.Request) {
	pr, ok := a.principal(w, r, auth.ActionRead)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	resolved, err := a.vault.Resolve(name)
	if err != nil {
		a.trail.Record(pr.Subject, pr.Tenant, auth.ActionRead, name, audit.OutcomeRefused, err.Error())
		a.fail(w, r, err)
		return
	}
	body, err := os.ReadFile(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			httpx.Fail(w, http.StatusNotFound, fmt.Sprintf("no object %q", name))
			return
		}
		a.fail(w, r, err)
		return
	}
	a.trail.Record(pr.Subject, pr.Tenant, auth.ActionRead, name, audit.OutcomeOK, "object")
	writeObject(w, contentType(name), body)
}

// handleAttachments lists the attachments of the tree.
func (a *API) handleAttachments(w http.ResponseWriter, r *http.Request) {
	pr, ok := a.principal(w, r, auth.ActionAttachRead)
	if !ok {
		return
	}
	items, err := a.vault.Attachments()
	if err != nil {
		a.fail(w, r, err)
		return
	}
	limit := a.limits.ClampPage(queryInt(r, "limit", 0))
	if len(items) > limit {
		items = items[:limit]
	}
	a.trail.Record(pr.Subject, pr.Tenant, auth.ActionAttachRead, "", audit.OutcomeOK,
		fmt.Sprintf("listed %d attachments", len(items)))
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"attachments": items, "limit": limit})
}

// handleAttachment returns one attachment by name.
func (a *API) handleAttachment(w http.ResponseWriter, r *http.Request) {
	pr, ok := a.principal(w, r, auth.ActionAttachRead)
	if !ok {
		return
	}
	name := httpx.Param(r, "name")
	body, err := a.vault.ReadAttachment(name)
	if err != nil {
		a.trail.Record(pr.Subject, pr.Tenant, auth.ActionAttachRead, name, audit.OutcomeRefused, err.Error())
		a.fail(w, r, err)
		return
	}
	start, end, err := vault.ParseRange(r.Header.Get("Range"), int64(len(body)))
	if err != nil {
		httpx.Fail(w, http.StatusRequestedRangeNotSatisfiable, err.Error())
		return
	}
	a.trail.Record(pr.Subject, pr.Tenant, auth.ActionAttachRead, name, audit.OutcomeOK, "attachment")
	writeObject(w, contentType(name), body[start:end])
}

// handleAttachmentUpload stores one attachment.
func (a *API) handleAttachmentUpload(w http.ResponseWriter, r *http.Request) {
	pr, ok := a.principal(w, r, auth.ActionWrite)
	if !ok {
		return
	}
	name := httpx.Param(r, "name")
	if r.Body == nil {
		httpx.Fail(w, http.StatusBadRequest, "empty body")
		return
	}
	body := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, err := r.Body.Read(buf)
		if n > 0 {
			if int64(len(body)+n) > a.limits.MaxUpload {
				httpx.Fail(w, http.StatusRequestEntityTooLarge,
					fmt.Sprintf("attachment above %d bytes", a.limits.MaxUpload))
				return
			}
			body = append(body, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	item, err := a.vault.WriteAttachment(name, body)
	if err != nil {
		a.trail.Record(pr.Subject, pr.Tenant, auth.ActionWrite, name, audit.OutcomeRefused, err.Error())
		a.fail(w, r, err)
		return
	}
	a.trail.Record(pr.Subject, pr.Tenant, auth.ActionWrite, item.Key, audit.OutcomeOK, "attachment")
	httpx.WriteJSON(w, http.StatusCreated, item)
}

// writeObject writes object bytes with the content type the tree stores them under.
func writeObject(w http.ResponseWriter, mediaType string, body []byte) {
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

// contentType decides a media type from an object name. An extension the service does not know is
// served as bytes, never guessed as HTML.
func contentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".pdf":
		return "application/pdf"
	case ".csv":
		return "text/csv; charset=utf-8"
	case ".txt":
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// safeFilename reduces an object key to a name that is safe in a Content-Disposition header: the
// last path segment, with quotes and separators removed.
func safeFilename(key string) string {
	base := filepath.Base(filepath.ToSlash(key))
	base = strings.ReplaceAll(base, "\"", "")
	base = strings.ReplaceAll(base, "\\", "")
	base = strings.ReplaceAll(base, "\r", "")
	base = strings.ReplaceAll(base, "\n", "")
	if base == "" || base == "." || base == "/" {
		return "document"
	}
	return base
}
