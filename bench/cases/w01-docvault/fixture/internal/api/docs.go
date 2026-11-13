package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"example.com/docvault/internal/audit"
	"example.com/docvault/internal/auth"
	"example.com/docvault/internal/httpx"
	"example.com/docvault/internal/vault"
)

// maxTitle is the longest title the API stores. A title longer than this is refused, not cut: a cut
// title is a title the caller did not send.
const maxTitle = 200

// docRequest is the body the create and update endpoints accept. The tenant is never read from it:
// a caller acts in the tenant its token names.
type docRequest struct {
	Name  string   `json:"name"`
	Title string   `json:"title"`
	Kind  string   `json:"kind"`
	Tags  []string `json:"tags"`
	Body  string   `json:"body"`
}

// handleList returns the documents of the caller's tenant, newest first.
func (a *API) handleList(w http.ResponseWriter, r *http.Request) {
	pr, ok := a.principal(w, r, auth.ActionRead)
	if !ok {
		return
	}
	limit := a.limits.ClampPage(queryInt(r, "limit", 0))
	offset := queryInt(r, "offset", 0)
	if offset < 0 {
		httpx.Fail(w, http.StatusBadRequest, "offset must not be negative")
		return
	}
	docs := a.vault.Store().List(vault.Query{
		Tenant: pr.Tenant,
		Kind:   strings.TrimSpace(r.URL.Query().Get("kind")),
		Tag:    strings.TrimSpace(r.URL.Query().Get("tag")),
		Prefix: strings.TrimSpace(r.URL.Query().Get("prefix")),
		Limit:  limit,
		Offset: offset,
	})
	a.trail.Record(pr.Subject, pr.Tenant, auth.ActionRead, "", audit.OutcomeOK,
		fmt.Sprintf("listed %d documents", len(docs)))
	if docs == nil {
		docs = []vault.Doc{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"documents": docs,
		"limit":     limit,
		"offset":    offset,
	})
}

// handleGet returns one document's metadata.
func (a *API) handleGet(w http.ResponseWriter, r *http.Request) {
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
	httpx.WriteJSON(w, http.StatusOK, doc)
}

// handleCreate stores one document and its object.
func (a *API) handleCreate(w http.ResponseWriter, r *http.Request) {
	pr, ok := a.principal(w, r, auth.ActionWrite)
	if !ok {
		return
	}
	var req docRequest
	if err := httpx.DecodeJSON(w, r, a.limits.MaxBody, &req); err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	doc, err := a.buildDoc(pr, req, "")
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := a.vault.Store().Get(doc.Key); err == nil {
		a.fail(w, r, fmt.Errorf("%w: %q", vault.ErrConflict, doc.Key))
		return
	}
	if err := a.vault.Write(doc.Key, []byte(req.Body)); err != nil {
		a.fail(w, r, err)
		return
	}
	doc.Size = int64(len(req.Body))
	doc = a.vault.Store().Put(doc)
	a.trail.Record(pr.Subject, pr.Tenant, auth.ActionWrite, doc.Key, audit.OutcomeOK, "created")
	httpx.WriteJSON(w, http.StatusCreated, doc)
}

// handleUpdate replaces the metadata and body of one document.
func (a *API) handleUpdate(w http.ResponseWriter, r *http.Request) {
	pr, ok := a.principal(w, r, auth.ActionWrite)
	if !ok {
		return
	}
	existing, err := a.docOf(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if !auth.InTenant(pr, existing.Tenant) {
		httpx.Fail(w, http.StatusNotFound, "no such document")
		return
	}
	var req docRequest
	if err := httpx.DecodeJSON(w, r, a.limits.MaxBody, &req); err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	doc, err := a.buildDoc(pr, req, existing.Key)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.vault.Write(doc.Key, []byte(req.Body)); err != nil {
		a.fail(w, r, err)
		return
	}
	doc.ID = existing.ID
	doc.Size = int64(len(req.Body))
	doc = a.vault.Store().Put(doc)
	a.trail.Record(pr.Subject, pr.Tenant, auth.ActionWrite, doc.Key, audit.OutcomeOK, "updated")
	httpx.WriteJSON(w, http.StatusOK, doc)
}

// handleDelete removes one document from the tree and the index.
func (a *API) handleDelete(w http.ResponseWriter, r *http.Request) {
	pr, ok := a.principal(w, r, auth.ActionDelete)
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
	if err := a.vault.Remove(doc.Key); err != nil {
		a.fail(w, r, err)
		return
	}
	a.trail.Record(pr.Subject, pr.Tenant, auth.ActionDelete, doc.Key, audit.OutcomeOK, "deleted")
	w.WriteHeader(http.StatusNoContent)
}

// buildDoc validates a request into a document. The object key comes from the vault's normalizer
// and is used only after the vault resolves it, so a name that would leave the tree is refused here
// before anything is written.
func (a *API) buildDoc(pr auth.Principal, req docRequest, key string) (vault.Doc, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return vault.Doc{}, fmt.Errorf("%w: a document needs a name", vault.ErrInvalid)
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return vault.Doc{}, fmt.Errorf("%w: a document needs a title", vault.ErrInvalid)
	}
	if len(title) > maxTitle {
		return vault.Doc{}, fmt.Errorf("%w: title longer than %d bytes", vault.ErrInvalid, maxTitle)
	}
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = "article"
	}
	if !vault.IsKind(kind) {
		return vault.Doc{}, fmt.Errorf("%w: kind %q", vault.ErrInvalid, kind)
	}
	if int64(len(req.Body)) > a.limits.MaxUpload {
		return vault.Doc{}, fmt.Errorf("%w: body above %d bytes", vault.ErrInvalid, a.limits.MaxUpload)
	}
	tags := make([]string, 0, len(req.Tags))
	for _, tag := range req.Tags {
		if tag = strings.TrimSpace(tag); tag != "" && len(tags) < 32 {
			tags = append(tags, tag)
		}
	}
	if key == "" {
		key = name
	}
	objectKey, err := a.vault.Key(key)
	if err != nil {
		return vault.Doc{}, err
	}
	return vault.Doc{
		ID:     docID(objectKey),
		Key:    objectKey,
		Tenant: pr.Tenant,
		Title:  title,
		Kind:   kind,
		Owner:  pr.Subject,
		Tags:   tags,
	}, nil
}

// docID derives the public identifier of a document from its key. It is stable across restarts
// because it is a function of the key alone.
func docID(key string) string {
	sum := 0
	for _, r := range key {
		sum = (sum*31 + int(r)) % 1000000
	}
	return fmt.Sprintf("doc-%06d", sum)
}

// queryInt reads an integer query parameter, falling back to def for an absent or malformed one.
func queryInt(r *http.Request, name string, def int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}

// queryBool reads a boolean query parameter.
func queryBool(r *http.Request, name string) bool {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get(name))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}
