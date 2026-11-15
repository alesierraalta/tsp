package shiptrack_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/shiptrack"
	"example.com/shiptrack/internal/config"
	"example.com/shiptrack/internal/identity"
)

// fixedNow is the instant the happy-path suite works around: shipments are created in May, the month is
// closed in June, and the events sit inside the ingest window.
var fixedNow = time.Date(2024, 5, 20, 10, 0, 0, 0, time.UTC)

// deployment is a service, its HTTP handler and the token the partners' console uses.
type deployment struct {
	service *shiptrack.Service
	server  *httptest.Server
	token   string
	now     time.Time
}

// openDeployment opens a service on a temporary root with one tenant, one partner, one carrier and the
// default rate book. Every test below starts from it, so the suite runs the code a deployment runs.
func openDeployment(t *testing.T, endpoint string) *deployment {
	t.Helper()
	root := t.TempDir()
	env := config.Env{}
	env.Set(config.KeyRoot, root)
	env.Set(config.KeySecret, "0123456789abcdef0123456789abcdef")
	env.Set(config.KeyTenants, "acme")
	env.Set(config.KeyZone, "UTC")
	env.Set(config.KeyTaxRate, "1900")
	env.Set(config.KeyMaxNote, "256")
	if endpoint != "" {
		env.Set(config.KeyEgress, "127.0.0.1")
		env.Set(config.KeyInsecure, "true")
	}
	cfg, err := config.Load(env)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	service, err := shiptrack.Open(cfg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	now := fixedNow
	service.SetClock(func() time.Time { return now })
	if _, err := service.RegisterPartner(identity.Partner{
		Tenant:   "acme",
		ID:       "shop",
		Name:     "Shop & Co <b>",
		Endpoint: endpoint,
	}); err != nil {
		t.Fatalf("register partner: %v", err)
	}
	if _, err := service.RegisterCarrier(identity.Carrier{
		ID:   "fastco",
		Name: "Fast Co",
		Lanes: []identity.Lane{
			{Origin: "mad", Destination: "bcn", Services: []string{"ground", "express"}},
		},
	}); err != nil {
		t.Fatalf("register carrier: %v", err)
	}
	server := httptest.NewServer(service.Handler(log.New(io.Discard, "", 0)))
	t.Cleanup(server.Close)
	token := service.Issue("partner:shop", "acme", []string{
		"events:write", "shipments:read", "shipments:write", "statements:read",
		"statements:close", "reports:read", "reports:export", "deliveries:write",
	})
	return &deployment{service: service, server: server, token: token, now: now}
}

// call performs one request against the service and returns the status and the body.
func (d *deployment) call(t *testing.T, method, path string, body any, accept string) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, d.server.URL+path, reader)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+d.token)
	if accept != "" {
		request.Header.Set("Accept", accept)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return response.StatusCode, payload
}

// createShipment books one shipment and returns its id.
func (d *deployment) createShipment(t *testing.T) string {
	t.Helper()
	status, body := d.call(t, http.MethodPost, "/api/shipments", map[string]any{
		"carrier": "fastco",
		"lane":    "mad>bcn",
		"service": "ground",
		"grams":   1500,
	}, "application/json")
	if status != http.StatusCreated {
		t.Fatalf("create shipment: status %d: %s", status, body)
	}
	var created struct {
		Shipment struct {
			ID string `json:"id"`
		} `json:"shipment"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("decode shipment: %v", err)
	}
	if created.Shipment.ID == "" {
		t.Fatalf("no shipment id in %s", body)
	}
	return created.Shipment.ID
}

// ingestScan posts one delivered scan for a shipment.
func (d *deployment) ingestScan(t *testing.T, shipment string) {
	t.Helper()
	batch := map[string]any{
		"count": 1,
		"events": []map[string]any{{
			"shipment":    shipment,
			"carrier":     "fastco",
			"origin":      "mad",
			"destination": "bcn",
			"service":     "ground",
			"kind":        "delivered",
			"at":          time.Date(2024, 5, 20, 8, 30, 0, 0, time.UTC).Format(time.RFC3339),
			"facility":    "bcn-hub",
			"grams":       1500,
			"note":        "left at the desk",
		}},
	}
	status, body := d.call(t, http.MethodPost, "/api/events", batch, "application/json")
	if status != http.StatusAccepted {
		t.Fatalf("ingest: status %d: %s", status, body)
	}
}

func TestShipmentIngestAndTimeline(t *testing.T) {
	d := openDeployment(t, "")
	id := d.createShipment(t)
	d.ingestScan(t, id)

	status, body := d.call(t, http.MethodGet, "/api/shipments/"+id, nil, "application/json")
	if status != http.StatusOK {
		t.Fatalf("show shipment: status %d: %s", status, body)
	}
	var shown struct {
		Timeline []struct {
			Kind     string `json:"kind"`
			Note     string `json:"note"`
			Facility string `json:"facility"`
		} `json:"timeline"`
	}
	if err := json.Unmarshal(body, &shown); err != nil {
		t.Fatalf("decode shipment: %v", err)
	}
	if len(shown.Timeline) != 1 || shown.Timeline[0].Kind != "delivered" {
		t.Fatalf("timeline = %+v", shown.Timeline)
	}
	if shown.Timeline[0].Note != "left at the desk" {
		t.Fatalf("note = %q", shown.Timeline[0].Note)
	}
}

func TestStatementCloseAndPage(t *testing.T) {
	d := openDeployment(t, "")
	id := d.createShipment(t)
	d.ingestScan(t, id)

	// The month the shipments were booked in is over by the time the close runs.
	d.service.SetClock(func() time.Time {
		return time.Date(2024, 6, 10, 9, 0, 0, 0, time.UTC)
	})
	status, body := d.call(t, http.MethodPost, "/api/close", map[string]any{"period": "2024-05"}, "application/json")
	if status != http.StatusCreated {
		t.Fatalf("close: status %d: %s", status, body)
	}
	var closed struct {
		Summary string `json:"summary"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(body, &closed); err != nil {
		t.Fatalf("decode close: %v", err)
	}
	if !strings.Contains(closed.Summary, "1 lines") {
		t.Fatalf("summary = %q", closed.Summary)
	}
	if !strings.HasPrefix(closed.Path, "partners/shop/statements/2024-05/") {
		t.Fatalf("path = %q", closed.Path)
	}

	status, page := d.call(t, http.MethodGet, "/api/statements/2024-05", nil, "text/html")
	if status != http.StatusOK {
		t.Fatalf("statement page: status %d: %s", status, page)
	}
	if !strings.Contains(string(page), "Shipment lines") {
		t.Fatalf("page does not carry the lines table: %s", page)
	}
	// The partner's display name carries markup; the page shows it as text.
	if !strings.Contains(string(page), "Shop &amp; Co &lt;b&gt;") {
		t.Fatalf("the partner name is not escaped: %s", page)
	}
	if _, err := os.Stat(filepath.Join(d.service.Root(), filepath.FromSlash(closed.Path))); err != nil {
		t.Fatalf("statement artifact: %v", err)
	}
}

func TestReportRunAndExport(t *testing.T) {
	d := openDeployment(t, "")
	id := d.createShipment(t)
	d.ingestScan(t, id)

	status, body := d.call(t, http.MethodGet, "/api/reports/events-by-carrier?kind=delivered", nil, "application/json")
	if status != http.StatusOK {
		t.Fatalf("report: status %d: %s", status, body)
	}
	var ran struct {
		Count int                 `json:"count"`
		SQL   string              `json:"sql"`
		Rows  []map[string]string `json:"rows"`
	}
	if err := json.Unmarshal(body, &ran); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if ran.Count != 1 {
		t.Fatalf("report count = %d", ran.Count)
	}
	if !strings.Contains(ran.SQL, "?") || strings.Contains(ran.SQL, "delivered") {
		t.Fatalf("the selection text must carry placeholders and no value: %q", ran.SQL)
	}
	if len(ran.Rows) != 1 || ran.Rows[0]["kind"] != "delivered" {
		t.Fatalf("rows = %+v", ran.Rows)
	}

	// An undeclared parameter is refused rather than ignored.
	status, body = d.call(t, http.MethodGet, "/api/reports/events-by-carrier?bogus=1", nil, "application/json")
	if status != http.StatusBadRequest {
		t.Fatalf("undeclared parameter: status %d: %s", status, body)
	}

	status, csv := d.call(t, http.MethodGet, "/api/export/events-by-carrier.csv", nil, "text/csv")
	if status != http.StatusOK {
		t.Fatalf("export: status %d: %s", status, csv)
	}
	if !strings.Contains(string(csv), "carrier,kind") || !strings.Contains(string(csv), "delivered") {
		t.Fatalf("export body: %s", csv)
	}
	if !strings.Contains(string(csv), "?") {
		t.Fatalf("export header does not carry the selection: %s", csv)
	}
}

func TestLabelRenderedThroughTheWorkspace(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "labelrender")
	body := "#!/bin/sh\n" +
		"in=\"\"; out=\"\"\n" +
		"while [ $# -gt 0 ]; do case \"$1\" in --input) in=\"$2\"; shift 2;; --output) out=\"$2\"; shift 2;; *) shift;; esac; done\n" +
		"printf '%%PDF-1.4 label for %s\\n' \"$in\" > \"$out\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write renderer: %v", err)
	}
	env := config.Env{}
	env.Set(config.KeyRoot, filepath.Join(root, "workspace"))
	env.Set(config.KeySecret, "0123456789abcdef0123456789abcdef")
	env.Set(config.KeyTenants, "acme")
	if err := os.MkdirAll(filepath.Join(root, "workspace"), 0o755); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	env.Set(config.KeyRenderer, script)
	cfg, err := config.Load(env)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	service, err := shiptrack.Open(cfg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	service.SetClock(func() time.Time { return fixedNow })
	if _, err := service.RegisterPartner(identity.Partner{Tenant: "acme", ID: "shop", Name: "Shop"}); err != nil {
		t.Fatalf("partner: %v", err)
	}
	if _, err := service.RegisterCarrier(identity.Carrier{ID: "fastco", Name: "Fast Co", Lanes: []identity.Lane{
		{Origin: "mad", Destination: "bcn", Services: []string{"ground"}},
	}}); err != nil {
		t.Fatalf("carrier: %v", err)
	}
	server := httptest.NewServer(service.Handler(log.New(io.Discard, "", 0)))
	defer server.Close()
	token := service.Issue("partner:shop", "acme", []string{"shipments:write", "shipments:read"})

	post := func(path string, body any) (int, []byte) {
		encoded, _ := json.Marshal(body)
		request, err := http.NewRequest(http.MethodPost, server.URL+path, bytes.NewReader(encoded))
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer response.Body.Close()
		payload, _ := io.ReadAll(response.Body)
		return response.StatusCode, payload
	}
	status, created := post("/api/shipments", map[string]any{
		"carrier": "fastco", "lane": "mad>bcn", "service": "ground", "grams": 2500,
	})
	if status != http.StatusCreated {
		t.Fatalf("shipment: status %d: %s", status, created)
	}
	var shipment struct {
		Shipment struct {
			ID string `json:"id"`
		} `json:"shipment"`
	}
	if err := json.Unmarshal(created, &shipment); err != nil {
		t.Fatalf("decode: %v", err)
	}
	status, rendered := post("/api/shipments/"+shipment.Shipment.ID+"/label", nil)
	if status != http.StatusCreated {
		t.Fatalf("label: status %d: %s", status, rendered)
	}
	var label struct {
		Bytes  int    `json:"bytes"`
		Label  string `json:"label"`
		Digest string `json:"digest"`
	}
	if err := json.Unmarshal(rendered, &label); err != nil {
		t.Fatalf("decode label: %v", err)
	}
	if label.Bytes <= 0 || len(label.Digest) != 64 {
		t.Fatalf("label = %+v", label)
	}
	if _, err := os.Stat(filepath.Join(service.Root(), filepath.FromSlash(label.Label))); err != nil {
		t.Fatalf("label artifact: %v", err)
	}
}

func TestDeliveryReachesThePartner(t *testing.T) {
	received := make(chan string, 1)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		received <- string(payload)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer hook.Close()

	d := openDeployment(t, hook.URL+"/hook")
	id := d.createShipment(t)
	d.ingestScan(t, id)
	d.service.SetClock(func() time.Time { return time.Date(2024, 6, 10, 9, 0, 0, 0, time.UTC) })
	if status, body := d.call(t, http.MethodPost, "/api/close", map[string]any{"period": "2024-05"}, "application/json"); status != http.StatusCreated {
		t.Fatalf("close: status %d: %s", status, body)
	}
	status, body := d.call(t, http.MethodPost, "/api/deliveries", map[string]any{"period": "2024-05"}, "application/json")
	if status != http.StatusAccepted {
		t.Fatalf("delivery: status %d: %s", status, body)
	}
	select {
	case payload := <-received:
		if !strings.Contains(payload, "statement.closed") || !strings.Contains(payload, "2024-05") {
			t.Fatalf("partner received %s", payload)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the partner received no notice")
	}
}

func TestTokenScopeIsExact(t *testing.T) {
	d := openDeployment(t, "")
	// A token that holds "statements:readonly" grants nothing that "statements:read" requires.
	readonly := d.service.Issue("partner:shop", "acme", []string{"statements:readonly"})
	request, err := http.NewRequest(http.MethodGet, d.server.URL+"/api/statements/2024-05", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+readonly)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.StatusCode)
	}
}

func TestHealthNeedsNoToken(t *testing.T) {
	d := openDeployment(t, "")
	response, err := http.Get(d.server.URL + "/healthz")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || !strings.Contains(string(payload), `"ok"`) {
		t.Fatalf("health = %d %s", response.StatusCode, payload)
	}
}
