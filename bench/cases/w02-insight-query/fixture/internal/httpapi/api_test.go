package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/insight"
)

const secret = "api-test-secret-0123456789"

func service(t *testing.T) *insight.Service {
	t.Helper()
	svc, err := insight.Open(insight.Options{TokenSecret: secret, Seed: true})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	svc.SetClock(func() time.Time { return time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC) })
	return svc
}

func get(t *testing.T, svc *insight.Service, token, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)
	return rec
}

type reportBody struct {
	Tenant string           `json:"tenant"`
	Count  int              `json:"count"`
	Rows   []map[string]any `json:"rows"`
}

func TestHealthIsPublic(t *testing.T) {
	svc := service(t)
	rec := get(t, svc, "", "/healthz")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("health: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestCatalogueNeedsTheReadScope(t *testing.T) {
	svc := service(t)
	rec := get(t, svc, svc.Issue("kelvin", "acme", []string{"reports:export"}, time.Hour), "/api/reports")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("catalogue without reports:read: status %d body %s", rec.Code, rec.Body.String())
	}
	rec = get(t, svc, svc.Issue("kelvin", "acme", []string{"reports:read"}, time.Hour), "/api/reports")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"usage"`) {
		t.Fatalf("catalogue: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestUsageReportAnswersForTheCallersTenant(t *testing.T) {
	svc := service(t)
	token := svc.Issue("kelvin", "acme", []string{"reports:read"}, time.Hour)
	rec := get(t, svc, token, "/api/reports/usage")
	if rec.Code != http.StatusOK {
		t.Fatalf("usage: status %d body %s", rec.Code, rec.Body.String())
	}
	var body reportBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Tenant != "acme" || body.Count != 4 {
		t.Fatalf("usage = %+v", body)
	}
	for _, row := range body.Rows {
		if row["tenant"] != "acme" {
			t.Fatalf("a row of another tenant leaked: %+v", row)
		}
	}
}

func TestStatusFilterNarrowsTheReport(t *testing.T) {
	svc := service(t)
	token := svc.Issue("kelvin", "acme", []string{"reports:read"}, time.Hour)
	rec := get(t, svc, token, "/api/reports/usage?status=archived")
	if rec.Code != http.StatusOK {
		t.Fatalf("filtered usage: status %d body %s", rec.Code, rec.Body.String())
	}
	var body reportBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Count != 1 || body.Rows[0]["status"] != "archived" {
		t.Fatalf("filtered usage = %+v", body)
	}
}

func TestUnknownFilterIsRefused(t *testing.T) {
	svc := service(t)
	token := svc.Issue("kelvin", "acme", []string{"reports:read"}, time.Hour)
	rec := get(t, svc, token, "/api/reports/quota?nope=1")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown filter: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestEventsEndpointRefusesAnUnknownStatus(t *testing.T) {
	svc := service(t)
	token := svc.Issue("kelvin", "acme", []string{"reports:read"}, time.Hour)
	rec := get(t, svc, token, "/api/events?status=booked'%20OR%20'1'%3D'1")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("injected status: status %d body %s", rec.Code, rec.Body.String())
	}
	rec = get(t, svc, token, "/api/events?status=booked")
	if rec.Code != http.StatusOK {
		t.Fatalf("booked events: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestExportIsCSVOfTheCallersTenant(t *testing.T) {
	svc := service(t)
	token := svc.Issue("kelvin", "acme", []string{"reports:export", "reports:read"}, time.Hour)
	rec := get(t, svc, token, "/api/reports/usage/export.csv")
	if rec.Code != http.StatusOK {
		t.Fatalf("export: status %d body %s", rec.Code, rec.Body.String())
	}
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("export has %d lines, want a header and four acme events: %s", len(lines), rec.Body.String())
	}
	if !strings.HasPrefix(lines[0], "id,tenant,account") {
		t.Fatalf("header = %q", lines[0])
	}
	for _, line := range lines[1:] {
		if !strings.Contains(line, ",acme,") {
			t.Fatalf("an exported row belongs to another tenant: %q", line)
		}
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "events.csv") {
		t.Fatalf("disposition = %q", rec.Header().Get("Content-Disposition"))
	}
}

func TestExportNeedsBothScopes(t *testing.T) {
	svc := service(t)
	rec := get(t, svc, svc.Issue("kelvin", "acme", []string{"reports:export"}, time.Hour), "/api/reports/usage/export.csv")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("export without reports:read: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestAccountsAndQuotaNeedTheirScopes(t *testing.T) {
	svc := service(t)
	rec := get(t, svc, svc.Issue("amara", "acme", []string{"accounts:read"}, time.Hour), "/api/accounts")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "kelvin") {
		t.Fatalf("accounts: status %d body %s", rec.Code, rec.Body.String())
	}
	rec = get(t, svc, svc.Issue("amara", "acme", []string{"reports:read"}, time.Hour), "/api/accounts")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("accounts without accounts:read: status %d body %s", rec.Code, rec.Body.String())
	}
	rec = get(t, svc, svc.Issue("amara", "acme", []string{"quotas:read"}, time.Hour), "/api/quota")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "kelvin") {
		t.Fatalf("quota: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestUnknownReportIsNotFound(t *testing.T) {
	svc := service(t)
	rec := get(t, svc, svc.Issue("kelvin", "acme", []string{"reports:read"}, time.Hour), "/api/reports/nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown report: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestUnsignedTokenIsUnauthorized(t *testing.T) {
	svc := service(t)
	req := httptest.NewRequest(http.MethodGet, "/api/reports", nil)
	req.Header.Set("Authorization", "Bearer v1|kelvin|acme|reports:read|9999999999")
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned token: status %d body %s", rec.Code, rec.Body.String())
	}
}
