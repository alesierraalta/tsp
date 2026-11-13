// Command triggers exercises the planted defects of this case against whichever module it is run
// from: `sh testdata/triggers/run.sh fixture` reads the fixture, and the same command with `all` or
// `keep-<ID>` reads that variant. The output is what KEY.json records as trigger.actual.
//
// The harness lives outside the fixture package, so it opens the service through its exported options
// and reads the answers the HTTP surface gives: it imports nothing the fixture keeps internal.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	billing "example.com/billing"
)

// secret is the signing secret the harness runs the service with.
const secret = "trigger-secret-0123456789"

// at is the instant the harness pins: the 24th of a thirty-day cycle, so seven days are left.
var at = time.Date(2024, 4, 24, 9, 0, 0, 0, time.UTC)

// subscription is the demonstration subscription the seat trigger is aimed at: it carries five seats
// on the starter plan, which includes two, so three seats are billable.
const subscription = "sub-000000000000000000000001"

// amount is an amount as a response carries it.
type amount struct {
	Minor    int64  `json:"minor"`
	Currency string `json:"currency"`
	Display  string `json:"display"`
}

// seatsData is the part of a seat change response the harness reads.
type seatsData struct {
	Charge amount `json:"charge"`
	Credit amount `json:"credit"`
	Change string `json:"change"`
}

// envelope is the body every response carries.
type envelope struct {
	OK    bool      `json:"ok"`
	Data  seatsData `json:"data"`
	Error string    `json:"error"`
}

func main() {
	service, err := billing.Open(billing.Options{
		Tenants:       []string{"acme", "globex"},
		TokenSecret:   secret,
		AdminSubjects: []string{"root"},
		Now:           at,
		Seed:          true,
	})
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	token, err := service.Issue("amara", "acme", []string{
		"billing:read", "billing:write", "reports:read", "reports:export",
	}, time.Hour)
	if err != nil {
		fmt.Println("issue:", err)
		os.Exit(1)
	}
	handler := service.Handler(nil)

	// D1: the export narrows the caller's usage events by the status the caller sends.
	injection := "archived' OR '1'='1"
	path := "/api/reports/usage/export.csv?status=" + url.QueryEscape(injection)
	rec := get(handler, token, path)
	if rec.Code == http.StatusOK {
		rows, tenants := csvShape(rec.Body.String())
		fmt.Printf("D1 GET /api/reports/usage/export.csv?status=%s -> status=%d rows=%d tenants=%v\n",
			injection, rec.Code, rows, tenants)
	} else {
		fmt.Printf("D1 GET /api/reports/usage/export.csv?status=%s -> status=%d error=%s\n",
			injection, rec.Code, errorOf(rec.Body.String()))
	}

	// The same export with a status the report declares answers the caller's tenant only.
	rec = get(handler, token, "/api/reports/usage/export.csv?status=recorded")
	if rows, tenants := csvShape(rec.Body.String()); rec.Code == http.StatusOK {
		fmt.Printf("ok GET /api/reports/usage/export.csv?status=recorded -> status=%d rows=%d tenants=%v\n",
			rec.Code, rows, tenants)
	} else {
		fmt.Printf("ok GET /api/reports/usage/export.csv?status=recorded -> status=%d error=%s\n",
			rec.Code, errorOf(rec.Body.String()))
	}

	// D2: five billable seats are added on the 24th day of a thirty-day cycle, so they are billed for
	// the seven days that are left.
	rec = post(handler, token, "/api/subscriptions/"+subscription+"/seats", `{"seats":10}`)
	var added envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &added); err != nil {
		fmt.Println("decode:", err)
		os.Exit(1)
	}
	if rec.Code == http.StatusOK {
		fmt.Printf("D2 POST /api/subscriptions/%s/seats {\"seats\":10} -> status=%d charge=%d minor (%s)\n",
			subscription, rec.Code, added.Data.Charge.Minor, added.Data.Charge.Display)
	} else {
		fmt.Printf("D2 POST /api/subscriptions/%s/seats {\"seats\":10} -> status=%d error=%s\n",
			subscription, rec.Code, added.Error)
	}

	// The same five seats are removed again: the credit is the shared split of the same whole-cycle
	// price over the same seven days, so it is what the charge above has to agree with.
	rec = post(handler, token, "/api/subscriptions/"+subscription+"/seats", `{"seats":5}`)
	var removed envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &removed); err != nil {
		fmt.Println("decode:", err)
		os.Exit(1)
	}
	if rec.Code == http.StatusOK {
		fmt.Printf("ok POST /api/subscriptions/%s/seats {\"seats\":5} -> status=%d credit=%d minor (%s)\n",
			subscription, rec.Code, removed.Data.Credit.Minor, removed.Data.Credit.Display)
	} else {
		fmt.Printf("ok POST /api/subscriptions/%s/seats {\"seats\":5} -> status=%d error=%s\n",
			subscription, rec.Code, removed.Error)
	}
}

// get performs one authenticated GET against the service.
func get(handler http.Handler, token, path string) *httptest.ResponseRecorder {
	return call(handler, token, http.MethodGet, path, "")
}

// post performs one authenticated POST against the service.
func post(handler http.Handler, token, path, body string) *httptest.ResponseRecorder {
	return call(handler, token, http.MethodPost, path, body)
}

// call performs one authenticated request against the service.
func call(handler http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// csvShape returns the number of rows a CSV body carries and the tenants its rows name, so one line of
// output says whether the export answered the caller's tenant or every tenant.
func csvShape(body string) (int, []string) {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) <= 1 {
		return 0, nil
	}
	header := strings.Split(lines[0], ",")
	tenantColumn := -1
	for i, name := range header {
		if name == "tenant" {
			tenantColumn = i
		}
	}
	if tenantColumn < 0 {
		return len(lines) - 1, nil
	}
	seen := make(map[string]bool, 4)
	for _, line := range lines[1:] {
		cells := strings.Split(line, ",")
		if tenantColumn < len(cells) {
			seen[cells[tenantColumn]] = true
		}
	}
	tenants := make([]string, 0, len(seen))
	for tenant := range seen {
		tenants = append(tenants, tenant)
	}
	sort.Strings(tenants)
	return len(lines) - 1, tenants
}

// errorOf returns the message of a refused response.
func errorOf(body string) string {
	var envelope struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return strings.TrimSpace(body)
	}
	return envelope.Error
}
