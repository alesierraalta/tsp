package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/docvault/internal/api"
	"example.com/docvault/internal/auth"
	"example.com/docvault/internal/config"
	"example.com/docvault/internal/vault"
)

const testSecret = "docvault-test-secret-0123456789"

type service struct {
	api    *api.API
	cfg    config.Config
	issuer *auth.Issuer
	vault  *vault.Vault
	root   string
}

func newService(t *testing.T) *service {
	t.Helper()
	root := t.TempDir()
	cfg := config.Config{
		Root:        root,
		Addr:        ":0",
		PageSize:    20,
		MaxUpload:   1 << 20,
		TokenSecret: testSecret,
		SessionTTL:  time.Hour,
		Tenants:     []string{"acme", "globex"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config: %v", err)
	}
	tree, err := vault.Open(root)
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}
	issuer, err := auth.NewIssuer(testSecret)
	if err != nil {
		t.Fatalf("issuer: %v", err)
	}
	svc := &service{api: api.New(cfg, tree, issuer), cfg: cfg, issuer: issuer, vault: tree, root: root}
	svc.api.SetClock(func() time.Time { return time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC) })
	return svc
}

func (s *service) token(t *testing.T, tenant string, scopes ...string) string {
	t.Helper()
	return s.issuer.Issue("amara", tenant, scopes, time.Hour)
}

func (s *service) do(t *testing.T, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	s.api.Handler(nil).ServeHTTP(rec, req)
	return rec
}

func (s *service) create(t *testing.T, tenant, name, title, kind, body string) vault.Doc {
	t.Helper()
	req := map[string]any{"name": name, "title": title, "kind": kind, "body": body}
	rec := s.do(t, s.token(t, tenant, "documents:write", "documents:read"), http.MethodPost, "/api/documents", req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s: status %d body %s", name, rec.Code, rec.Body.String())
	}
	var doc vault.Doc
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode created document: %v", err)
	}
	return doc
}

func TestHealthIsPublic(t *testing.T) {
	s := newService(t)
	rec := s.do(t, "", http.MethodGet, "/healthz", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("health: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestListRequiresTheReadScope(t *testing.T) {
	s := newService(t)
	rec := s.do(t, s.token(t, "acme", "documents:write"), http.MethodGet, "/api/documents", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("list without documents:read: status %d body %s", rec.Code, rec.Body.String())
	}
	rec = s.do(t, s.token(t, "acme", "documents:read"), http.MethodGet, "/api/documents", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list with documents:read: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestScopesAreExact(t *testing.T) {
	s := newService(t)
	tok := s.token(t, "acme", "documents:readonly")
	rec := s.do(t, tok, http.MethodGet, "/api/documents", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a scope that merely extends documents:read granted a read: status %d body %s",
			rec.Code, rec.Body.String())
	}
}

func TestBadTokenIsUnauthorized(t *testing.T) {
	s := newService(t)
	req := httptest.NewRequest(http.MethodGet, "/api/documents", nil)
	req.Header.Set("Authorization", "Bearer not-a-token")
	rec := httptest.NewRecorder()
	s.api.Handler(nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned token: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestCreateAndReadADocument(t *testing.T) {
	s := newService(t)
	doc := s.create(t, "acme", "documents/notes/standup.txt", "Standup notes", "note", "one\ntwo\n")
	if doc.Tenant != "acme" || doc.Owner != "amara" {
		t.Fatalf("created document carries %+v", doc)
	}
	if _, err := os.Stat(filepath.Join(s.root, "documents/notes/standup.txt")); err != nil {
		t.Fatalf("the object is not on disk: %v", err)
	}

	rec := s.do(t, s.token(t, "acme", "documents:read"), http.MethodGet, "/api/documents/"+doc.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: status %d body %s", rec.Code, rec.Body.String())
	}
	var got vault.Doc
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Title != "Standup notes" || got.Kind != "note" {
		t.Fatalf("get returned %+v", got)
	}

	rec = s.do(t, s.token(t, "acme", "documents:read"), http.MethodGet, "/api/documents", nil)
	if !strings.Contains(rec.Body.String(), doc.ID) {
		t.Fatalf("the list does not carry the document: %s", rec.Body.String())
	}
}

func TestCreateRefusesANameThatLeavesTheRoot(t *testing.T) {
	s := newService(t)
	req := map[string]any{"name": "../escape.txt", "title": "escape", "kind": "note", "body": "x"}
	rec := s.do(t, s.token(t, "acme", "documents:write"), http.MethodPost, "/api/documents", req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("creating ../escape.txt: status %d body %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(s.root), "escape.txt")); err == nil {
		t.Fatal("the create wrote outside the root")
	}
}

func TestDocumentOfAnotherTenantIsNotFound(t *testing.T) {
	s := newService(t)
	doc := s.create(t, "acme", "documents/acme.txt", "Acme", "article", "body")
	rec := s.do(t, s.token(t, "globex", "documents:read"), http.MethodGet, "/api/documents/"+doc.ID, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant get: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestDownloadOfADocument(t *testing.T) {
	s := newService(t)
	doc := s.create(t, "acme", "documents/report.txt", "Report", "article", "quarterly numbers")
	rec := s.do(t, s.token(t, "acme", "documents:read"), http.MethodGet, "/api/documents/"+doc.ID+"/download", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("download: status %d body %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "quarterly numbers" {
		t.Fatalf("download body = %q", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "report.txt") {
		t.Fatalf("download disposition = %q", got)
	}
}

func TestObjectEndpointRefusesATraversalName(t *testing.T) {
	s := newService(t)
	doc := s.create(t, "acme", "documents/report.txt", "Report", "article", "body")
	rec := s.do(t, s.token(t, "acme", "documents:read"), http.MethodGet,
		"/api/documents/"+doc.ID+"/object?name=../outside.txt", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("object ../outside.txt: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestThumbnailServesAnObjectOfTheTree(t *testing.T) {
	s := newService(t)
	doc := s.create(t, "acme", "documents/pic.png", "Picture", "article", "png-bytes")
	if err := s.vault.Write("documents/pic-thumb.png", []byte("thumb-bytes")); err != nil {
		t.Fatalf("write thumbnail: %v", err)
	}
	rec := s.do(t, s.token(t, "acme", "attachments:read"),
		http.MethodGet, "/api/documents/"+doc.ID+"/thumbnail?name=documents/pic-thumb.png", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("thumbnail: status %d body %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "thumb-bytes" {
		t.Fatalf("thumbnail body = %q", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("thumbnail content type = %q", got)
	}
}

func TestPreviewEscapesArticleText(t *testing.T) {
	s := newService(t)
	doc := s.create(t, "acme", "documents/markup.txt", "<b>bold</b>", "article", "<i>body</i>")
	rec := s.do(t, s.token(t, "acme", "documents:read"), http.MethodGet, "/api/documents/"+doc.ID+"/preview", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: status %d body %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "<b>bold</b>") || strings.Contains(body, "<i>body</i>") {
		t.Fatalf("preview wrote raw document text: %s", body)
	}
	if !strings.Contains(body, "&lt;b&gt;bold&lt;/b&gt;") {
		t.Fatalf("preview does not carry the escaped title: %s", body)
	}
}

func TestPreviewRendersANote(t *testing.T) {
	s := newService(t)
	doc := s.create(t, "acme", "documents/standup.txt", "Standup notes", "note", "line one")
	rec := s.do(t, s.token(t, "acme", "documents:read"), http.MethodGet, "/api/documents/"+doc.ID+"/preview", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<h1>Standup notes</h1>") {
		t.Fatalf("preview lost the note heading: %s", rec.Body.String())
	}
}

func TestAttachmentRoundTrip(t *testing.T) {
	s := newService(t)
	tok := s.token(t, "acme", "attachments:read", "documents:write")
	req := httptest.NewRequest(http.MethodPost, "/api/attachments/logo.png", strings.NewReader("png-data"))
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	s.api.Handler(nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload attachment: status %d body %s", rec.Code, rec.Body.String())
	}

	rec = s.do(t, s.token(t, "acme", "attachments:read"), http.MethodGet, "/api/attachments/logo.png", nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "png-data" {
		t.Fatalf("read attachment: status %d body %q", rec.Code, rec.Body.String())
	}

	rec = s.do(t, s.token(t, "acme", "attachments:read"), http.MethodGet, "/api/attachments", nil)
	if !strings.Contains(rec.Body.String(), "logo.png") {
		t.Fatalf("attachment list = %s", rec.Body.String())
	}
}

func TestAttachmentNameMustBeFlat(t *testing.T) {
	s := newService(t)
	rec := s.do(t, s.token(t, "acme", "attachments:read"), http.MethodGet, "/api/attachments/..%5Csecret.txt", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("nested attachment name: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestExportDefusesSpreadsheetFormulas(t *testing.T) {
	s := newService(t)
	s.create(t, "acme", "documents/ledger.txt", "=SUM(A1:A9)", "article", "body")
	rec := s.do(t, s.token(t, "acme", "documents:export", "documents:read"), http.MethodGet, "/api/export.csv", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("export: status %d body %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), ",=SUM(A1:A9),") {
		t.Fatalf("export left a formula live: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "'=SUM(A1:A9)") {
		t.Fatalf("export does not carry the defused cell: %s", rec.Body.String())
	}
}

func TestAdminRoutesNeedTheOperatorScope(t *testing.T) {
	s := newService(t)
	rec := s.do(t, s.token(t, "acme", "documents:read"), http.MethodGet, "/api/admin/stats", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("stats with a reader scope: status %d body %s", rec.Code, rec.Body.String())
	}
	rec = s.do(t, s.token(t, "acme", "admin:operate"), http.MethodGet, "/api/admin/stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats with the operator scope: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestWritesAreAudited(t *testing.T) {
	s := newService(t)
	doc := s.create(t, "acme", "documents/audited.txt", "Audited", "article", "body")
	rec := s.do(t, s.token(t, "acme", "audit:read"), http.MethodGet, "/api/admin/audit", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("audit: status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), doc.Key) {
		t.Fatalf("audit trail does not carry the write: %s", rec.Body.String())
	}
}

func TestPageSizeIsCapped(t *testing.T) {
	s := newService(t)
	for i := 0; i < 5; i++ {
		s.create(t, "acme", "documents/doc-"+string(rune('a'+i))+".txt", "Doc", "article", "body")
	}
	rec := s.do(t, s.token(t, "acme", "documents:read"), http.MethodGet, "/api/documents?limit=1000", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"limit":20`) {
		t.Fatalf("a request above the configured page size was not capped: %s", rec.Body.String())
	}
}
