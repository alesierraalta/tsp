// Command triggers exercises the planted defects of this case against whichever module it is run
// from: `sh testdata/triggers/run.sh fixture` reads the fixture, and the same command with `all` or
// `keep-<ID>` reads that variant. The output is what KEY.json records as trigger.actual.
//
// The harness lives outside the module, so it writes the staging tree with the on-disk format the
// README documents instead of calling the service's own encoders.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	archive "example.com/archive"
)

const secret = "trigger-secret-0123456789"

// batchID is the identifier of the interrupted batch the harness stages. It has the shape a minted
// batch id has, so the staging tree accepts it like any other.
const batchID = "bat-00112233445566778899aabb"

func main() {
	dir, err := os.MkdirTemp("", "archive-trigger-")
	if err != nil {
		fmt.Println("mkdtemp:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)
	root := filepath.Join(dir, "archive")
	service, err := archive.Open(archive.Options{
		Root:        root,
		Tenants:     []string{"acme"},
		PageSize:    20,
		MaxUpload:   1 << 20,
		MaxEntries:  20,
		MaxPages:    50,
		TokenSecret: secret,
		SessionTTL:  time.Hour,
	})
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	service.SetClock(func() time.Time { return time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC) })

	// A page image outside the archive root, so a name that walks out of the tree has something to read.
	outside := filepath.Join(dir, "outside.pgm")
	outsideBody := archive.PGMPage("outside-secret-page")
	if err := os.WriteFile(outside, outsideBody, 0o644); err != nil {
		fmt.Println("write outside:", err)
		os.Exit(1)
	}

	if err := stageInterrupted(root, outsideBody); err != nil {
		fmt.Println("stage:", err)
		os.Exit(1)
	}

	// D1: the resume rebuilds the pages of an interrupted batch from its page index.
	result, err := service.Resume(batchID)
	outcome := pageLine(result.Text("scan-01"), 2)
	if err != nil {
		outcome = "err=" + err.Error()
	}
	for _, entry := range result.Entries {
		if entry.Err != "" {
			outcome = "err=" + entry.Err
		}
	}
	fmt.Printf("D1 resume scan-01 page 2 -> %s\n", outcome)

	// The guarded sibling reads the same page index through the staging layout.
	handler := service.Handler(nil)
	token := service.Issue("amara", "acme", []string{"imports:read"}, time.Hour)
	rec := call(handler, token, http.MethodGet, "/api/batches/"+batchID+"/pages/2?entry=scan-01")
	fmt.Printf("ok GET /api/batches/%s/pages/2?entry=scan-01 -> status=%d body=%q\n",
		batchID, rec.Code, strings.TrimSpace(rec.Body.String()))

	// D2: the import report renders an index entry's note.
	imported, err := importBatch(service)
	if err != nil {
		fmt.Println("import:", err)
		os.Exit(1)
	}
	rec = call(handler, token, http.MethodGet, "/api/batches/"+imported.Batch+"/report")
	fmt.Printf("D2 GET /api/batches/%s/report index note -> %s\n", imported.Batch, indexNote(rec.Body.String()))

	// The same report escapes the note of every other kind of entry.
	page := rec.Body.String()
	fmt.Printf("ok GET /api/batches/%s/report -> raw notes=%d escaped notes=%d\n",
		imported.Batch,
		strings.Count(page, "<script>alert(1)</script>"),
		strings.Count(page, "&lt;script&gt;alert(1)&lt;/script&gt;"))
}

// stageInterrupted writes the staging tree of a batch that stopped after the scanner delivered it: its
// manifest, its container, one page, and the page index the scanner wrote.
func stageInterrupted(root string, outsideBody []byte) error {
	layout := filepath.Join(root, ".staging", batchID)
	entryDir := filepath.Join(layout, "scans", "scan-01")
	if err := os.MkdirAll(entryDir, 0o755); err != nil {
		return err
	}
	container := archive.PDF("scan-01", []string{"one", "two"})
	manifest := fmt.Sprintf(`{
  "id": %q,
  "tenant": "acme",
  "created": "2024-04-30T09:00:00Z",
  "entries": [
    {"name": "scan-01", "kind": "scan", "title": "Letter from the assessor", "pages": 2, "bytes": %d}
  ]
}
`, batchID, len(container))
	if err := os.WriteFile(filepath.Join(layout, "manifest.json"), []byte(manifest), 0o644); err != nil {
		return err
	}
	page := archive.PGMPage("resident page one")
	index := fmt.Sprintf("# page\tfile\tdigest\n001\tpage-001.pgm\t%s\n002\t../../../../../outside.pgm\t%s\n",
		digest(page), digest(outsideBody))
	for name, body := range map[string][]byte{
		"scan-01.pdf":    container,
		"page-001.pgm":   page,
		"page-index.tsv": []byte(index),
	} {
		if err := os.WriteFile(filepath.Join(entryDir, name), body, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// importedBatch is the id of the batch the harness imports for the report trigger.
type importedBatch struct{ Batch string }

// importBatch delivers one batch through the service: a scan with a plain note and an index entry whose
// note is markup the sender wrote.
func importBatch(service *archive.Service) (importedBatch, error) {
	type entry struct {
		Name  string `json:"name"`
		Kind  string `json:"kind"`
		Title string `json:"title,omitempty"`
		Pages int    `json:"pages,omitempty"`
		Note  string `json:"note,omitempty"`
	}
	container := archive.PDF("scan-02", []string{"First page"})
	index := []byte("row,column\n1,2\n")
	m := struct {
		Tenant  string  `json:"tenant"`
		Created string  `json:"created"`
		Entries []entry `json:"entries"`
	}{
		Tenant:  "acme",
		Created: "2024-04-30T10:00:00Z",
		Entries: []entry{
			{Name: "scan-02", Kind: "scan", Title: "Intake sheet", Pages: 1, Note: "<script>alert(1)</script>"},
			{Name: "listing.csv", Kind: "index", Note: "<script>alert(1)</script>"},
		},
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return importedBatch{}, err
	}
	delivered, err := service.ImportManifest(raw,
		map[string][]byte{"scan-02": container, "listing.csv": index},
		map[string]map[int][]byte{"scan-02": {1: archive.PGMPage("intake sheet")}})
	if err != nil {
		return importedBatch{}, err
	}
	return importedBatch{Batch: delivered.Batch}, nil
}

// call performs one authenticated GET against the service.
func call(handler http.Handler, token, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(nil))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// pageLine returns the extraction line of one page, so the output is one line whether the text came from
// the batch's own tree or from outside it.
func pageLine(text string, page int) string {
	prefix := fmt.Sprintf("[page %03d] ", page)
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return "(no page line)"
}

// indexNote returns the content of the report's index-note block.
func indexNote(page string) string {
	return between(page, `<pre class="index-note">`, "</pre>")
}

// between returns the text between two markers, or "(none)" when the page does not carry them.
func between(page, open, close string) string {
	start := strings.Index(page, open)
	if start < 0 {
		return "(none)"
	}
	rest := page[start+len(open):]
	end := strings.Index(rest, close)
	if end < 0 {
		return "(unterminated)"
	}
	return rest[:end]
}

// digest renders the SHA-256 of body the way a page index carries it.
func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
