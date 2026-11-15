package docvault_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	docvault "example.com/docvault"
)

func open(t *testing.T, opts docvault.Options) *docvault.Service {
	t.Helper()
	if opts.Root == "" {
		opts.Root = t.TempDir()
	}
	if opts.TokenSecret == "" {
		opts.TokenSecret = "service-test-secret-0123456789"
	}
	if len(opts.Tenants) == 0 {
		opts.Tenants = []string{"acme"}
	}
	svc, err := docvault.Open(opts)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return svc
}

func TestOpenValidatesTheOptions(t *testing.T) {
	if _, err := docvault.Open(docvault.Options{Root: "/", TokenSecret: "0123456789abcdef", Tenants: []string{"acme"}}); err == nil {
		t.Fatal("a service on the filesystem root was opened")
	}
	if _, err := docvault.Open(docvault.Options{Root: t.TempDir(), TokenSecret: "short", Tenants: []string{"acme"}}); err == nil {
		t.Fatal("a service without a usable secret was opened")
	}
	if _, err := docvault.Open(docvault.Options{Root: t.TempDir(), TokenSecret: "0123456789abcdef"}); err == nil {
		t.Fatal("a service without a tenant was opened")
	}
}

func TestOpenUsesTheDefaults(t *testing.T) {
	svc := open(t, docvault.Options{})
	if svc.Addr() != ":8080" {
		t.Fatalf("addr = %q", svc.Addr())
	}
	if len(svc.Routes()) < 10 {
		t.Fatalf("routes = %v", svc.Routes())
	}
}

func TestSeedingAndServingAnObject(t *testing.T) {
	svc := open(t, docvault.Options{})
	if err := svc.WriteObject("documents/seed.txt", []byte("seed")); err != nil {
		t.Fatalf("write object: %v", err)
	}
	if _, err := os.Stat(filepath.Join(svc.Root(), "documents/seed.txt")); err != nil {
		t.Fatalf("the seeded object is not on disk: %v", err)
	}
	if err := svc.WriteObject("../escape.txt", []byte("x")); err == nil {
		t.Fatal("seeding a name outside the root was accepted")
	}
}

func TestIssuedTokenIsAcceptedByTheHandler(t *testing.T) {
	svc := open(t, docvault.Options{})
	svc.SetClock(func() time.Time { return time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC) })
	token := svc.Issue("amara", "acme", []string{"documents:read"}, time.Hour)

	req := httptest.NewRequest(http.MethodGet, "/api/documents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	svc.Handler(nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list with an issued token: status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"documents":[]`) {
		t.Fatalf("list body = %s", rec.Body.String())
	}
}

func TestHealthNeedsNoToken(t *testing.T) {
	svc := open(t, docvault.Options{})
	rec := httptest.NewRecorder()
	svc.Handler(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("health: status %d body %s", rec.Code, rec.Body.String())
	}
}
