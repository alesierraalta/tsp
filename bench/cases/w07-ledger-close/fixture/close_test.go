package ledgerclose_test

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

	"example.com/ledgerclose"
	"example.com/ledgerclose/internal/config"
	"example.com/ledgerclose/internal/identity"
	"example.com/ledgerclose/internal/pricing"
)

const tokenSecret = "close-test-secret-0123456789"

// clock is the instant the tests run at: June, so the close of May is a finished month.
func clock() time.Time { return time.Date(2024, 6, 10, 9, 0, 0, 0, time.UTC) }

func testConfig(t *testing.T, mutate func(*config.Config)) config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.Root = t.TempDir()
	cfg.Tenants = []string{"acme", "globex"}
	cfg.TokenSecret = tokenSecret
	cfg.MaxLines = 100
	if mutate != nil {
		mutate(&cfg)
	}
	return cfg
}

func open(t *testing.T, mutate func(*config.Config)) *ledgerclose.Service {
	t.Helper()
	service, err := ledgerclose.Open(testConfig(t, mutate))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	service.SetClock(clock)
	if err := service.AddRate("standard", pricing.Rate{
		Metric: "api_call",
		Tiers:  []pricing.Tier{{UpTo: 1_000, BasisPoints: 2_500}},
	}); err != nil {
		t.Fatalf("rate: %v", err)
	}
	accounts := []identity.Account{
		{
			ID: "team-api", Tenant: "acme", Subject: "amara", DisplayName: "Team API <b>live</b>",
			Email: "billing@example.com", Currency: "USD", RateTable: "standard",
		},
		{
			ID: "team-web", Tenant: "globex", Subject: "wren", DisplayName: "Team Web",
			Email: "billing@globex.example", Currency: "USD", RateTable: "standard",
		},
	}
	for _, account := range accounts {
		if _, err := service.Register(account); err != nil {
			t.Fatalf("register %s: %v", account.ID, err)
		}
	}
	return service
}

func call(t *testing.T, service *ledgerclose.Service, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	service.Handler(nil).ServeHTTP(recorder, request)
	return recorder
}

// usageBatch is one account's month of metered calls: ten calls at 1.00, rated at 25%.
func usageBatch() map[string]any {
	return map[string]any{
		"count": 1,
		"events": []map[string]any{
			{
				"id": "evt-0001-abcd", "account": "team-api", "metric": "api_call",
				"quantity": 10, "unit_amount": 100, "currency": "USD",
				"occurred_at": "2024-05-10T12:00:00Z",
			},
		},
	}
}

func TestOpenValidatesTheConfiguration(t *testing.T) {
	cases := map[string]func(*config.Config){
		"root at the filesystem root": func(cfg *config.Config) { cfg.Root = "/" },
		"secret too short":            func(cfg *config.Config) { cfg.TokenSecret = "short" },
		"no tenant":                   func(cfg *config.Config) { cfg.Tenants = nil },
		"page size above the ceiling": func(cfg *config.Config) { cfg.PageSize = 10_000 },
		"renderer is not executable":  func(cfg *config.Config) { cfg.Renderer = "/etc/hostname" },
	}
	for name, mutate := range cases {
		if _, err := ledgerclose.Open(testConfig(t, mutate)); err == nil {
			t.Fatalf("%s was opened", name)
		}
	}
}

func TestIngestCloseAndServeTheStatement(t *testing.T) {
	renderer := writeRenderer(t)
	service := open(t, func(cfg *config.Config) { cfg.Renderer = renderer })
	token := service.Issue("amara", "acme", []string{"usage:write", "usage:read", "close:run", "invoices:read", "invoices:export"})

	if rec := call(t, service, http.MethodPost, "/api/usage", token, usageBatch()); rec.Code != http.StatusCreated {
		t.Fatalf("ingest: status %d body %s", rec.Code, rec.Body.String())
	}
	if rec := call(t, service, http.MethodPost, "/api/close", token, map[string]any{"period": "2024-05"}); rec.Code != http.StatusOK {
		t.Fatalf("close: status %d body %s", rec.Code, rec.Body.String())
	} else if !strings.Contains(rec.Body.String(), `"count":1`) {
		t.Fatalf("close body = %s", rec.Body.String())
	}

	statementID := "inv-acme-team-api-2024-05"
	rec := call(t, service, http.MethodGet, "/api/invoices/"+statementID+"/statement", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("statement: status %d body %s", rec.Code, rec.Body.String())
	}
	page := rec.Body.String()
	if !strings.Contains(page, "Team API &lt;b&gt;live&lt;/b&gt;") {
		t.Fatalf("the statement did not escape the display name:\n%s", page)
	}
	if !strings.Contains(page, "2.50 USD") {
		t.Fatalf("the statement did not carry the net:\n%s", page)
	}

	rendered := call(t, service, http.MethodGet, "/api/invoices/"+statementID+"/statement.pdf", token, nil)
	if rendered.Code != http.StatusOK {
		t.Fatalf("statement.pdf: status %d body %s", rendered.Code, rendered.Body.String())
	}
	if !strings.HasPrefix(rendered.Body.String(), "%PDF-") {
		t.Fatalf("the rendered statement is not a PDF: %q", rendered.Body.String())
	}

	statementPath := filepath.Join(service.Root(), "tenants/acme/statements/2024-05/team-api/statement.html")
	if _, err := os.Stat(statementPath); err != nil {
		t.Fatalf("the statement is not in the workspace: %v", err)
	}

	exported := call(t, service, http.MethodGet, "/api/export/usage.csv", token, nil)
	if exported.Code != http.StatusOK {
		t.Fatalf("export: status %d body %s", exported.Code, exported.Body.String())
	}
	if !strings.HasPrefix(exported.Body.String(), "id,account,metric,quantity,unit_amount,currency,occurred_at") {
		t.Fatalf("the export header = %q", strings.SplitN(exported.Body.String(), "\n", 2)[0])
	}
	if !strings.Contains(exported.Body.String(), "team-api") {
		t.Fatalf("the export does not carry the account:\n%s", exported.Body.String())
	}
	if service.TrailLen() < 3 {
		t.Fatalf("the trail carries %d entries", service.TrailLen())
	}
}

func TestTheStatementNeedsTheTenantOfTheToken(t *testing.T) {
	service := open(t, nil)
	acme := service.Issue("amara", "acme", []string{"usage:write", "close:run", "invoices:read"})
	globex := service.Issue("wren", "globex", []string{"usage:write", "close:run", "invoices:read"})
	if rec := call(t, service, http.MethodPost, "/api/usage", acme, usageBatch()); rec.Code != http.StatusCreated {
		t.Fatalf("ingest: status %d body %s", rec.Code, rec.Body.String())
	}
	if rec := call(t, service, http.MethodPost, "/api/close", acme, map[string]any{"period": "2024-05"}); rec.Code != http.StatusOK {
		t.Fatalf("close: status %d body %s", rec.Code, rec.Body.String())
	}
	if rec := call(t, service, http.MethodPost, "/api/close", globex, map[string]any{"period": "2024-05"}); rec.Code != http.StatusOK {
		t.Fatalf("the other tenant's close: status %d body %s", rec.Code, rec.Body.String())
	}
	if rec := call(t, service, http.MethodGet, "/api/invoices/inv-acme-team-api-2024-05/statement", globex, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("another tenant read a statement: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestTokenAndScopeChecks(t *testing.T) {
	service := open(t, nil)
	reader := service.Issue("amara", "acme", []string{"usage:read"})

	if rec := call(t, service, http.MethodPost, "/api/usage", "", usageBatch()); rec.Code != http.StatusUnauthorized {
		t.Fatalf("an unauthenticated ingest: status %d", rec.Code)
	}
	if rec := call(t, service, http.MethodPost, "/api/usage", reader+"x", usageBatch()); rec.Code != http.StatusUnauthorized {
		t.Fatalf("a token with a broken signature: status %d", rec.Code)
	}
	writer := service.Issue("amara", "acme", []string{"usage:write"})
	if rec := call(t, service, http.MethodPost, "/api/close", writer, map[string]any{"period": "2024-05"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a close without the scope: status %d body %s", rec.Code, rec.Body.String())
	}
	if rec := call(t, service, http.MethodPost, "/api/usage", writer, usageBatch()); rec.Code != http.StatusCreated {
		t.Fatalf("the same token could not ingest: status %d body %s", rec.Code, rec.Body.String())
	}

	expiring := service.Issue("amara", "acme", []string{"usage:write"})
	service.SetClock(func() time.Time { return clock().Add(48 * time.Hour) })
	if rec := call(t, service, http.MethodPost, "/api/usage", expiring, usageBatch()); rec.Code != http.StatusUnauthorized {
		t.Fatalf("an expired token: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestIngestRefusesWhatItCannotBill(t *testing.T) {
	service := open(t, nil)
	token := service.Issue("amara", "acme", []string{"usage:write"})

	if rec := call(t, service, http.MethodPost, "/api/usage", token, usageBatch()); rec.Code != http.StatusCreated {
		t.Fatalf("ingest: status %d body %s", rec.Code, rec.Body.String())
	}
	if rec := call(t, service, http.MethodPost, "/api/usage", token, usageBatch()); rec.Code != http.StatusConflict {
		t.Fatalf("a repeated event id: status %d body %s", rec.Code, rec.Body.String())
	}

	stale := usageBatch()
	stale["events"].([]map[string]any)[0]["id"] = "evt-0002-abcd"
	stale["events"].([]map[string]any)[0]["occurred_at"] = "2024-05-10T12:00:00Z"
	stale["events"].([]map[string]any)[0]["account"] = "team-web"
	if rec := call(t, service, http.MethodPost, "/api/usage", token, stale); rec.Code != http.StatusBadRequest {
		t.Fatalf("an event of another tenant: status %d body %s", rec.Code, rec.Body.String())
	}

	overstated := usageBatch()
	overstated["count"] = 4
	if rec := call(t, service, http.MethodPost, "/api/usage", token, overstated); rec.Code != http.StatusBadRequest {
		t.Fatalf("a count the body does not back: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestTheExportPageIsCutFromTheTenantsOwnRows(t *testing.T) {
	service := open(t, func(cfg *config.Config) { cfg.PageSize = 1 })
	acme := service.Issue("amara", "acme", []string{"usage:write", "invoices:export", "invoices:read"})
	globex := service.Issue("wren", "globex", []string{"usage:write"})

	// The other tenant's event is the later one, so a page of one taken before the tenant filter is its row.
	if rec := call(t, service, http.MethodPost, "/api/usage", acme, usageBatch()); rec.Code != http.StatusCreated {
		t.Fatalf("ingest: status %d body %s", rec.Code, rec.Body.String())
	}
	theirs := usageBatch()
	theirs["events"].([]map[string]any)[0]["id"] = "evt-0002-abcd"
	theirs["events"].([]map[string]any)[0]["account"] = "team-web"
	theirs["events"].([]map[string]any)[0]["occurred_at"] = "2024-05-12T12:00:00Z"
	if rec := call(t, service, http.MethodPost, "/api/usage", globex, theirs); rec.Code != http.StatusCreated {
		t.Fatalf("the other tenant's ingest: status %d body %s", rec.Code, rec.Body.String())
	}

	rec := call(t, service, http.MethodGet, "/api/export/usage.csv?sort=occurred&desc=true&limit=1", acme, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("export: status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "team-api") {
		t.Fatalf("the caller's own row was not exported:\n%s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "team-web") {
		t.Fatalf("another tenant's row was exported:\n%s", rec.Body.String())
	}
}

func TestTheStreamedBatchFormIsAccepted(t *testing.T) {
	service := open(t, nil)
	token := service.Issue("amara", "acme", []string{"usage:write", "close:run"})
	body := `{"count":1}` + "\n" +
		`{"id":"evt-0009-abcd","account":"team-api","metric":"api_call","quantity":10,"unit_amount":100,"currency":"USD","occurred_at":"2024-05-10T12:00:00Z"}` + "\n"
	request := httptest.NewRequest(http.MethodPost, "/api/usage", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/x-ndjson")
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	service.Handler(nil).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("streamed ingest: status %d body %s", recorder.Code, recorder.Body.String())
	}
	if rec := call(t, service, http.MethodPost, "/api/close", token, map[string]any{"period": "2024-05"}); rec.Code != http.StatusOK {
		t.Fatalf("close after a streamed ingest: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestCloseRefusesAPeriodOutsideTheWindow(t *testing.T) {
	service := open(t, nil)
	token := service.Issue("amara", "acme", []string{"usage:write", "close:run"})
	if rec := call(t, service, http.MethodPost, "/api/usage", token, usageBatch()); rec.Code != http.StatusCreated {
		t.Fatalf("ingest: status %d body %s", rec.Code, rec.Body.String())
	}
	if rec := call(t, service, http.MethodPost, "/api/close", token, map[string]any{"period": "2024-06"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a close of the running month: status %d body %s", rec.Code, rec.Body.String())
	}
	if rec := call(t, service, http.MethodPost, "/api/close", token, map[string]any{"period": "2023-12"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a close before the window: status %d body %s", rec.Code, rec.Body.String())
	}
	if rec := call(t, service, http.MethodPost, "/api/close", token, map[string]any{"period": "2024-13"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a period outside 1..12: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestDeliveryReachesAPartnerEndpoint(t *testing.T) {
	delivered := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := new(bytes.Buffer)
		body.ReadFrom(r.Body)
		delivered <- body.String()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	service := open(t, func(cfg *config.Config) {
		cfg.Partners = []string{"127.0.0.1"}
		cfg.DeliverInsecure = true
	})
	token := service.Issue("amara", "acme", []string{"usage:write", "close:run", "deliveries:write"})
	if rec := call(t, service, http.MethodPost, "/api/usage", token, usageBatch()); rec.Code != http.StatusCreated {
		t.Fatalf("ingest: status %d body %s", rec.Code, rec.Body.String())
	}
	if rec := call(t, service, http.MethodPost, "/api/close", token, map[string]any{"period": "2024-05"}); rec.Code != http.StatusOK {
		t.Fatalf("close: status %d body %s", rec.Code, rec.Body.String())
	}
	rec := call(t, service, http.MethodPost, "/api/deliveries", token, map[string]any{
		"invoice":  "inv-acme-team-api-2024-05",
		"endpoint": server.URL + "/hooks",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("delivery: status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"delivered":true`) {
		t.Fatalf("delivery body = %s", rec.Body.String())
	}
	if payload := <-delivered; !strings.Contains(payload, "inv-acme-team-api-2024-05") {
		t.Fatalf("the partner received %q", payload)
	}

	refused := call(t, service, http.MethodPost, "/api/deliveries", token, map[string]any{
		"invoice":  "inv-acme-team-api-2024-05",
		"endpoint": "https://evil.example/hooks",
	})
	if refused.Code != http.StatusBadRequest {
		t.Fatalf("a delivery to a host off the list: status %d body %s", refused.Code, refused.Body.String())
	}
}

// writeRenderer writes a renderer that reads the HTML file and writes a small PDF. It is the process the
// close starts, and it takes the same argument vector a deployment's renderer takes.
func writeRenderer(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "render")
	script := "#!/bin/sh\n" +
		"while [ $# -gt 0 ]; do\n" +
		"  case \"$1\" in\n" +
		"    --input) src=\"$2\"; shift 2;;\n" +
		"    --output) dst=\"$2\"; shift 2;;\n" +
		"    *) shift;;\n" +
		"  esac\n" +
		"done\n" +
		"[ -f \"$src\" ] || exit 3\n" +
		"printf '%%PDF-1.4\\nstatement of %s\\n' \"$src\" > \"$dst\"\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write renderer: %v", err)
	}
	return path
}
