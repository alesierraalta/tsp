package archive_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	archive "example.com/archive"
	"example.com/archive/internal/manifest"
)

const secret = "test-secret-0123456789"

// open builds a service over a fresh root and pins its clock, which is what a deployment and a test both
// do before serving a request.
func open(t *testing.T) *archive.Service {
	t.Helper()
	root := filepath.Join(t.TempDir(), "archive")
	service, err := archive.Open(archive.Options{
		Root:        root,
		Tenants:     []string{"acme", "globex"},
		PageSize:    10,
		MaxUpload:   1 << 20,
		MaxEntries:  20,
		MaxPages:    50,
		TokenSecret: secret,
		SessionTTL:  time.Hour,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	service.SetClock(func() time.Time { return time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC) })
	return service
}

// token mints a token with the scopes a test needs.
func token(t *testing.T, service *archive.Service, tenant string, scopes ...string) string {
	t.Helper()
	return service.Issue("amara", tenant, scopes, time.Hour)
}

// delivery is one small batch a test imports: a scan with two pages and an index listing.
func delivery() (manifest.Manifest, map[string][]byte, map[string]map[int][]byte) {
	container := archive.PDF("scan-01", []string{"First page", "Second page"})
	index := []byte("row,column\n1,2\n")
	pages := map[string]map[int][]byte{
		"scan-01": {
			1: archive.PGMPage("First page of the letter"),
			2: archive.PGMPage("Second page of the letter"),
		},
	}
	m := manifest.Manifest{
		Tenant:  "acme",
		Created: "2024-04-30T09:00:00Z",
		Entries: []manifest.Entry{
			{Name: "scan-01", Kind: manifest.KindScan, Title: "Letter from the assessor", Pages: 2, Bytes: int64(len(container))},
			{Name: "listing.csv", Kind: manifest.KindIndex, Note: "12 rows, 4 columns"},
		},
	}
	containers := map[string][]byte{"scan-01": container, "listing.csv": index}
	return m, containers, pages
}

// request performs one authenticated request against the service's handler.
func request(t *testing.T, handler http.Handler, tok, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// decode reads a JSON response into a map.
func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return out
}

func TestImportCommitsObjectsAndExtractsText(t *testing.T) {
	service := open(t)
	m, containers, pages := delivery()
	result, err := service.ImportWithPages(m, containers, pages)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if !result.Committed || len(result.Entries) != 2 {
		t.Fatalf("result = %+v", result)
	}
	records := service.Documents("acme", 0)
	if len(records) != 2 {
		t.Fatalf("documents = %d, want 2", len(records))
	}
	var scan string
	for _, record := range records {
		if record.Kind == manifest.KindScan {
			scan = record.ID
		}
	}
	if scan == "" {
		t.Fatal("the scan was not committed")
	}
	text, err := service.DocumentText(scan)
	if err != nil {
		t.Fatalf("text: %v", err)
	}
	for _, want := range []string{"page 001", "First page of the letter", "Second page of the letter"} {
		if !strings.Contains(text, want) {
			t.Fatalf("extracted text %q does not carry %q", text, want)
		}
	}
}

func TestListingIsTenantScoped(t *testing.T) {
	service := open(t)
	m, containers, pages := delivery()
	if _, err := service.ImportWithPages(m, containers, pages); err != nil {
		t.Fatalf("import: %v", err)
	}
	handler := service.Handler(nil)
	other := token(t, service, "globex", "documents:read")
	rec := request(t, handler, other, http.MethodGet, "/api/documents")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	if docs, ok := body["documents"].([]any); !ok || len(docs) != 0 {
		t.Fatalf("globex sees %v, want no documents", body["documents"])
	}
	mine := token(t, service, "acme", "documents:read")
	rec = request(t, handler, mine, http.MethodGet, "/api/documents")
	body = decode(t, rec)
	if docs, ok := body["documents"].([]any); !ok || len(docs) != 2 {
		t.Fatalf("acme sees %v, want two documents", body["documents"])
	}
}

func TestScopeComparisonIsExact(t *testing.T) {
	service := open(t)
	handler := service.Handler(nil)
	narrow := token(t, service, "acme", "documents:readonly")
	rec := request(t, handler, narrow, http.MethodGet, "/api/documents")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d body %s, want 403", rec.Code, rec.Body.String())
	}
}

func TestExportPrefixesFormulaCells(t *testing.T) {
	service := open(t)
	m, containers, pages := delivery()
	m.Entries[0].Title = "=SUM(A1:A9)"
	if _, err := service.ImportWithPages(m, containers, pages); err != nil {
		t.Fatalf("import: %v", err)
	}
	handler := service.Handler(nil)
	tok := token(t, service, "acme", "documents:read", "documents:export")
	rec := request(t, handler, tok, http.MethodGet, "/api/export.csv")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "'=SUM(A1:A9)") {
		t.Fatalf("export does not keep the formula as text:\n%s", rec.Body.String())
	}
}

func TestReportRendersEntryValuesAsText(t *testing.T) {
	service := open(t)
	m, containers, pages := delivery()
	m.Entries[0].Title = "<b>bold title</b>"
	if _, err := service.ImportWithPages(m, containers, pages); err != nil {
		t.Fatalf("import: %v", err)
	}
	handler := service.Handler(nil)
	tok := token(t, service, "acme", "imports:read")
	path := "/api/batches/" + lastResult(t, service).Batch + "/report"
	rec := request(t, handler, tok, http.MethodGet, path)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	page := rec.Body.String()
	if strings.Contains(page, "<b>bold title</b>") {
		t.Fatalf("the title was rendered as markup:\n%s", page)
	}
	for _, want := range []string{"&lt;b&gt;bold title&lt;/b&gt;", "12 rows, 4 columns"} {
		if !strings.Contains(page, want) {
			t.Fatalf("report does not carry %q:\n%s", want, page)
		}
	}
}

// result is the last import's result, for a test that needs the batch id.
func lastResult(t *testing.T, service *archive.Service) archiveResult {
	t.Helper()
	records := service.Documents("acme", 0)
	if len(records) == 0 {
		t.Fatal("nothing committed")
	}
	return archiveResult{Batch: records[0].Batch}
}

// archiveResult keeps the one field the tests read from an import result.
type archiveResult struct{ Batch string }

func TestDownloadServesTheContainer(t *testing.T) {
	service := open(t)
	m, containers, pages := delivery()
	if _, err := service.ImportWithPages(m, containers, pages); err != nil {
		t.Fatalf("import: %v", err)
	}
	handler := service.Handler(nil)
	tok := token(t, service, "acme", "documents:read")
	for _, record := range service.Documents("acme", 0) {
		if record.Kind != manifest.KindScan {
			continue
		}
		rec := request(t, handler, tok, http.MethodGet, "/api/documents/"+record.ID+"/download")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		body, err := io.ReadAll(rec.Body)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if string(body) != string(containers["scan-01"]) {
			t.Fatalf("download = %q", body)
		}
	}
}

func TestDeleteRemovesTheObject(t *testing.T) {
	service := open(t)
	m, containers, pages := delivery()
	if _, err := service.ImportWithPages(m, containers, pages); err != nil {
		t.Fatalf("import: %v", err)
	}
	handler := service.Handler(nil)
	tok := token(t, service, "acme", "documents:delete", "documents:read")
	record := service.Documents("acme", 0)[0]
	rec := request(t, handler, tok, http.MethodDelete, "/api/documents/"+record.ID)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if _, err := service.Document(record.ID); err == nil {
		t.Fatal("the object is still there")
	}
}

func TestAdminStatsNeedTheAdminScope(t *testing.T) {
	service := open(t)
	handler := service.Handler(nil)
	reader := token(t, service, "acme", "documents:read")
	if rec := request(t, handler, reader, http.MethodGet, "/api/admin/stats"); rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", rec.Code)
	}
	admin := token(t, service, "acme", "admin:operate")
	rec := request(t, handler, admin, http.MethodGet, "/api/admin/stats")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	if body["objects"].(float64) != 0 {
		t.Fatalf("stats = %v", body)
	}
}

func TestHealthNeedsNoToken(t *testing.T) {
	service := open(t)
	rec := request(t, service.Handler(nil), "", http.MethodGet, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestUnknownEndpointIsNotFound(t *testing.T) {
	service := open(t)
	rec := request(t, service.Handler(nil), "", http.MethodGet, "/api/nothing")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestManifestIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	service := open(t)
	m, containers, pages := delivery()
	m.Entries[0].Name = "../escape"
	if _, err := service.ImportWithPages(m, containers, pages); err == nil {
		t.Fatal("a name that walks upwards was accepted")
	}
	if got := service.Documents("acme", 0); len(got) != 0 {
		t.Fatalf("refused batch committed %d objects", len(got))
	}
}

func TestProviderRoundTrip(t *testing.T) {
	// The service's options are read back through Config, which is what the console prints.
	service := open(t)
	cfg := service.Config()
	if cfg.TenantList() != "acme,globex" {
		t.Fatalf("tenants = %q", cfg.TenantList())
	}
	if !cfg.Tenant("acme") || cfg.Tenant("ACME") {
		t.Fatal("tenant comparison is not exact")
	}
	if _, err := os.Stat(filepath.Join(service.Root(), "objects", "meta")); err != nil {
		t.Fatalf("store is not laid out: %v", err)
	}
}

// ExampleImport documents the shape of a delivery, which is also what the README's quick start runs.
func ExampleService_Import() {
	m := manifest.Manifest{
		Tenant:  "acme",
		Created: "2024-04-30T09:00:00Z",
		Entries: []manifest.Entry{{Name: "scan-01", Kind: manifest.KindScan, Pages: 1}},
	}
	fmt.Println(m.SummaryLine())
	// Output: 1 entries, 1 scan, 1 pages
}
