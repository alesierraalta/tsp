package billing_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	billing "example.com/billing"
)

// secret is the signing secret the tests run with.
const secret = "billing-test-secret-0123456789"

// seedNow is the instant the demonstration dataset is built around.
var seedNow = time.Date(2024, 4, 24, 9, 0, 0, 0, time.UTC)

// open returns a seeded service with a pinned clock.
func open(t *testing.T) *billing.Service {
	t.Helper()
	service, err := billing.Open(billing.Options{
		Tenants:       []string{"acme", "globex"},
		TokenSecret:   secret,
		Currency:      "EUR",
		AdminSubjects: []string{"root"},
		Now:           seedNow,
		Seed:          true,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return service
}

// token signs a token for the tests.
func token(t *testing.T, service *billing.Service, subject, tenant string, scopes ...string) string {
	t.Helper()
	signed, err := service.Issue(subject, tenant, scopes, time.Hour)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	return signed
}

// call performs one request against the service.
func call(t *testing.T, service *billing.Service, bearer, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	service.Handler(nil).ServeHTTP(rec, req)
	return rec
}

// decode reads a response body into a map.
func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		OK    bool           `json:"ok"`
		Data  map[string]any `json:"data"`
		Error string         `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	if !envelope.OK {
		t.Fatalf("response is a refusal: %q", envelope.Error)
	}
	return envelope.Data
}

func TestHealthzNeedsNoToken(t *testing.T) {
	service := open(t)
	rec := call(t, service, "", http.MethodGet, "/healthz", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestListingsAreScopedToTheTokensTenant(t *testing.T) {
	service := open(t)
	acme := token(t, service, "amara", "acme", "billing:read")
	globex := token(t, service, "bruno", "globex", "billing:read")

	data := decode(t, call(t, service, acme, http.MethodGet, "/api/subscriptions", ""))
	if data["tenant"] != "acme" {
		t.Fatalf("tenant = %v", data["tenant"])
	}
	if count := len(data["subscriptions"].([]any)); count != 2 {
		t.Fatalf("acme subscriptions = %d, want 2", count)
	}
	data = decode(t, call(t, service, globex, http.MethodGet, "/api/subscriptions", ""))
	if count := len(data["subscriptions"].([]any)); count != 1 {
		t.Fatalf("globex subscriptions = %d, want 1", count)
	}
	data = decode(t, call(t, service, acme, http.MethodGet, "/api/plans", ""))
	for _, raw := range data["plans"].([]any) {
		plan := raw.(map[string]any)
		if plan["code"] == "" {
			t.Fatalf("plan without a code: %v", plan)
		}
	}
}

func TestARecordOfAnotherTenantIsNotFound(t *testing.T) {
	service := open(t)
	acme := token(t, service, "amara", "acme", "billing:read")
	const globexSubscription = "sub-000000000000000000000003"
	rec := call(t, service, acme, http.MethodGet, "/api/subscriptions/"+globexSubscription, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestReportNarrowsByADeclaredFilter(t *testing.T) {
	service := open(t)
	bearer := token(t, service, "amara", "acme", "reports:read")
	data := decode(t, call(t, service, bearer, http.MethodGet, "/api/reports/usage?status=recorded", ""))
	rows := data["rows"].([]any)
	if len(rows) == 0 {
		t.Fatal("the recorded usage report answered no rows")
	}
	for _, raw := range rows {
		row := raw.([]any)
		if row[5] != "recorded" {
			t.Fatalf("row %v is not recorded", row)
		}
	}
	if data["tenant"] != "acme" {
		t.Fatalf("tenant = %v", data["tenant"])
	}
}

func TestReportRefusesAParameterItDoesNotDeclare(t *testing.T) {
	service := open(t)
	bearer := token(t, service, "amara", "acme", "reports:read")
	rec := call(t, service, bearer, http.MethodGet, "/api/reports/usage?colour=red", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestReportRefusesAValueOutsideTheAcceptedSet(t *testing.T) {
	service := open(t)
	bearer := token(t, service, "amara", "acme", "reports:read")
	rec := call(t, service, bearer, http.MethodGet, "/api/reports/usage?status=deleted", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestExportCarriesTheTenantsRows(t *testing.T) {
	service := open(t)
	bearer := token(t, service, "amara", "acme", "reports:read", "reports:export")
	rec := call(t, service, bearer, http.MethodGet, "/api/reports/usage/export.csv?status=recorded", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	if got := lines[0]; !strings.HasPrefix(got, "id,tenant,account") {
		t.Fatalf("header = %q", got)
	}
	if len(lines) != 4 {
		t.Fatalf("export carries %d lines, want a header and three rows:\n%s", len(lines), rec.Body.String())
	}
	for _, line := range lines[1:] {
		if !strings.Contains(line, ",acme,") {
			t.Fatalf("row %q is not the caller's tenant", line)
		}
		if !strings.Contains(line, ",recorded,") {
			t.Fatalf("row %q is not recorded", line)
		}
	}
}

func TestExportNeedsBothScopes(t *testing.T) {
	service := open(t)
	bearer := token(t, service, "amara", "acme", "reports:read")
	rec := call(t, service, bearer, http.MethodGet, "/api/reports/usage/export.csv", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestSeatChangeAtTheCycleStartChargesTheWholePrice(t *testing.T) {
	service := open(t)
	service.SetClock(time.Date(2024, 4, 1, 9, 0, 0, 0, time.UTC))
	bearer := token(t, service, "amara", "acme", "billing:write", "billing:read")
	const subscription = "sub-000000000000000000000001"
	rec := call(t, service, bearer, http.MethodPost, "/api/subscriptions/"+subscription+"/seats", `{"seats":8}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	data := decode(t, rec)
	charge := data["charge"].(map[string]any)
	// Five seats carried, two included, so three were billable before; eight seats make six billable.
	// Three billable seats at 10.00 EUR for a whole cycle is 30.00 EUR.
	if minor := charge["minor"].(float64); minor != 3000 {
		t.Fatalf("charge = %v minor, want 3000", charge["minor"])
	}
	view := data["subscription"].(map[string]any)
	if seats := view["seats"].(float64); seats != 8 {
		t.Fatalf("seats = %v, want 8", view["seats"])
	}
	if data["change"] != "added" {
		t.Fatalf("change = %v", data["change"])
	}
}

func TestPaymentSettlesTheInvoiceAndIsAudited(t *testing.T) {
	service := open(t)
	bearer := token(t, service, "amara", "acme", "invoices:write", "invoices:read", "audit:read")
	const invoice = "inv-000000000000000000000001"
	rec := call(t, service, bearer, http.MethodPost, "/api/invoices/"+invoice+"/payments",
		`{"amount":{"minor":7900,"currency":"EUR"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	data := decode(t, rec)
	view := data["invoice"].(map[string]any)
	if view["status"] != "paid" {
		t.Fatalf("status = %v, want paid", view["status"])
	}
	audit := decode(t, call(t, service, bearer, http.MethodGet, "/api/admin/audit", ""))
	if audit["count"].(float64) == 0 {
		t.Fatal("the trail is empty after a mutation")
	}
	records := audit["records"].([]any)
	found := false
	for _, raw := range records {
		record := raw.(map[string]any)
		if record["action"] == "invoices.write" && record["outcome"] == "accepted" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the accepted payment is not in the trail: %v", records)
	}
}

func TestPaymentAboveTheTotalIsRefused(t *testing.T) {
	service := open(t)
	bearer := token(t, service, "bruno", "globex", "invoices:write")
	const invoice = "inv-000000000000000000000003"
	rec := call(t, service, bearer, http.MethodPost, "/api/invoices/"+invoice+"/payments",
		`{"amount":{"minor":700000,"currency":"EUR"}}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestUsageIsRecordedAgainstTheActiveSubscription(t *testing.T) {
	service := open(t)
	bearer := token(t, service, "amara", "acme", "usage:write")
	const account = "acc-000000000000000000000001"
	rec := call(t, service, bearer, http.MethodPost, "/api/usage",
		`{"account":"`+account+`","metric":"requests","quantity":25}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	data := decode(t, rec)
	if data["quantity"].(float64) != 25 {
		t.Fatalf("quantity = %v", data["quantity"])
	}
	if data["metric"] != "requests" {
		t.Fatalf("metric = %v", data["metric"])
	}
	rec = call(t, service, bearer, http.MethodPost, "/api/usage",
		`{"account":"`+account+`","metric":"storage","quantity":5}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a metric the plan does not meter was accepted: %d %s", rec.Code, rec.Body.String())
	}
}

func TestATokenIsVerifiedBeforeAnythingElse(t *testing.T) {
	service := open(t)
	rec := call(t, service, "", http.MethodGet, "/api/subscriptions", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: status = %d", rec.Code)
	}
	rec = call(t, service, "not-a-token", http.MethodGet, "/api/subscriptions", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("malformed token: status = %d", rec.Code)
	}
	readonly := token(t, service, "amara", "acme", "billing:readonly")
	rec = call(t, service, readonly, http.MethodGet, "/api/subscriptions", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a provisional scope granted an action: %d", rec.Code)
	}
}

func TestTheOperatorRoutesNeedAnOperatorSubject(t *testing.T) {
	service := open(t)
	amara := token(t, service, "amara", "acme", "admin:operate")
	rec := call(t, service, amara, http.MethodGet, "/api/admin/console", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a subject the deployment does not name as an operator read the console: %d", rec.Code)
	}
	root := token(t, service, "root", "acme", "admin:operate")
	rec = call(t, service, root, http.MethodGet, "/api/admin/console", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	data := decode(t, rec)
	if strings.Contains(data["config"].(string), secret) {
		t.Fatal("the console page carries the signing secret")
	}
}

func TestASubscriptionsInvoicesAreListedByStatus(t *testing.T) {
	service := open(t)
	bearer := token(t, service, "amara", "acme", "invoices:read")
	data := decode(t, call(t, service, bearer, http.MethodGet, "/api/invoices?status=paid", ""))
	if count := len(data["invoices"].([]any)); count != 1 {
		t.Fatalf("paid invoices = %d, want 1", count)
	}
	rec := call(t, service, bearer, http.MethodGet, "/api/invoices?status=deleted", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("an unknown invoice status was accepted: %d", rec.Code)
	}
}
