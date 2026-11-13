// Command triggers exercises the planted defects of this case against whichever module it is run
// from: `cd fixture && go run ../testdata/triggers/main.go` reads the fixture, and the same command
// run from fix/all or fix/keep-<ID> reads that variant. The output is what KEY.json records as
// trigger.actual.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	docvault "example.com/docvault"
)

const secret = "trigger-secret-0123456789"

func main() {
	dir, err := os.MkdirTemp("", "docvault-trigger-")
	if err != nil {
		fmt.Println("mkdtemp:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)
	root := filepath.Join(dir, "vault")
	if err := os.MkdirAll(root, 0o755); err != nil {
		fmt.Println("mkdir:", err)
		os.Exit(1)
	}
	// An object outside the root, so a path that leaves the tree has something to read.
	if err := os.WriteFile(filepath.Join(dir, "outside.txt"), []byte("outside-secret"), 0o644); err != nil {
		fmt.Println("write outside:", err)
		os.Exit(1)
	}

	service, err := docvault.Open(docvault.Options{
		Root:        root,
		Tenants:     []string{"acme"},
		PageSize:    20,
		MaxUpload:   1 << 20,
		TokenSecret: secret,
		SessionTTL:  time.Hour,
	})
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	service.SetClock(func() time.Time { return time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC) })
	handler := service.Handler(nil)
	token := service.Issue("amara", "acme",
		[]string{"documents:read", "documents:write", "attachments:read"}, time.Hour)

	note, err := create(handler, token, "documents/note.txt", "<script>alert(1)</script>", "note", "")
	if err != nil {
		fmt.Println("create note:", err)
		os.Exit(1)
	}
	article, err := create(handler, token, "documents/article.txt", "<script>alert(1)</script>", "article", "")
	if err != nil {
		fmt.Println("create article:", err)
		os.Exit(1)
	}

	// D1: the thumbnail endpoint is handed a name that walks out of the tree.
	rec := call(handler, token, http.MethodGet, "/api/documents/"+note.ID+"/thumbnail?name=../outside.txt", nil)
	fmt.Printf("D1 GET /api/documents/%s/thumbnail?name=../outside.txt -> status=%d body=%q\n",
		note.ID, rec.Code, strings.TrimSpace(rec.Body.String()))

	// D2: a note with no body is previewed; its title becomes the page heading.
	rec = call(handler, token, http.MethodGet, "/api/documents/"+note.ID+"/preview", nil)
	fmt.Printf("D2 GET /api/documents/%s/preview note title <script>alert(1)</script> -> %s\n",
		note.ID, heading(rec.Body.String()))

	// Siblings that do guard the same input, so the run shows the defect is one entry point.
	rec = call(handler, token, http.MethodGet, "/api/documents/"+note.ID+"/object?name=../outside.txt", nil)
	fmt.Printf("ok GET /api/documents/%s/object?name=../outside.txt -> status=%d body=%q\n",
		note.ID, rec.Code, strings.TrimSpace(rec.Body.String()))

	rec = call(handler, token, http.MethodGet, "/api/documents/"+article.ID+"/preview", nil)
	fmt.Printf("ok GET /api/documents/%s/preview article title <script>alert(1)</script> -> %s\n",
		article.ID, heading(rec.Body.String()))
}

func create(handler http.Handler, token, name, title, kind, body string) (struct{ ID string }, error) {
	var doc struct{ ID string }
	rec := call(handler, token, http.MethodPost, "/api/documents", map[string]any{
		"name": name, "title": title, "kind": kind, "body": body,
	})
	if rec.Code != http.StatusCreated {
		return doc, fmt.Errorf("status %d body %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		return doc, err
	}
	return doc, nil
}

func call(handler http.Handler, token, method, path string, body any) *httptest.ResponseRecorder {
	var payload io.Reader = bytes.NewReader(nil)
	if body != nil {
		raw, _ := json.Marshal(body)
		payload = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, payload)
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// heading returns the page's h1 so the output is one line whether the title was escaped or not.
func heading(page string) string {
	start := strings.Index(page, "<h1>")
	if start < 0 {
		return "heading=<none>"
	}
	rest := page[start+len("<h1>"):]
	end := strings.Index(rest, "</h1>")
	if end < 0 {
		return "heading=<unterminated>"
	}
	return "heading=" + rest[:end]
}
