package auth_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/docvault/internal/auth"
)

const secret = "policy-test-secret-0123456789"

func issuer(t *testing.T) *auth.Issuer {
	t.Helper()
	i, err := auth.NewIssuer(secret)
	if err != nil {
		t.Fatalf("issuer: %v", err)
	}
	i.SetClock(func() time.Time { return time.Date(2024, 5, 1, 9, 0, 0, 0, time.UTC) })
	return i
}

func TestTokenRoundTrip(t *testing.T) {
	i := issuer(t)
	raw := i.Issue("amara", "acme", []string{"documents:read", "documents:write"}, time.Hour)
	tok, err := i.Verify(raw)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if tok.Subject != "amara" || tok.Tenant != "acme" || len(tok.Scopes) != 2 {
		t.Fatalf("token = %+v", tok)
	}
	if got := tok.Expires.Format(time.RFC3339); got != "2024-05-01T10:00:00Z" {
		t.Fatalf("expiry = %s", got)
	}
}

func TestTamperedTokenIsRefused(t *testing.T) {
	i := issuer(t)
	raw := i.Issue("amara", "acme", []string{"documents:read"}, time.Hour)
	tampered := strings.Replace(raw, "acme", "globex", 1)
	if _, err := i.Verify(tampered); !errors.Is(err, auth.ErrSignature) {
		t.Fatalf("tampered token: err = %v, want ErrSignature", err)
	}
}

func TestExpiredTokenIsRefused(t *testing.T) {
	i := issuer(t)
	raw := i.Issue("amara", "acme", []string{"documents:read"}, -time.Minute)
	if _, err := i.Verify(raw); !errors.Is(err, auth.ErrExpired) {
		t.Fatalf("expired token: err = %v, want ErrExpired", err)
	}
}

func TestMalformedTokens(t *testing.T) {
	i := issuer(t)
	for _, raw := range []string{"", "no-signature", "v1|a|b|c|1."} {
		if _, err := i.Verify(raw); err == nil {
			t.Fatalf("token %q was accepted", raw)
		}
	}
}

func TestBearerHeader(t *testing.T) {
	if got, err := auth.Bearer("Bearer abc.def"); err != nil || got != "abc.def" {
		t.Fatalf("bearer = %q, %v", got, err)
	}
	for _, header := range []string{"", "Basic abc", "Bearer "} {
		if _, err := auth.Bearer(header); err == nil {
			t.Fatalf("header %q was accepted", header)
		}
	}
}

func TestPolicyIsExactScopeMembership(t *testing.T) {
	policy := auth.NewPolicy()
	reader := auth.Principal{Subject: "amara", Tenant: "acme", Scopes: auth.NewScopeSet([]string{"documents:read"})}
	if !policy.Allows(reader, auth.ActionRead) {
		t.Fatal("documents:read does not allow documents:read")
	}
	if policy.Allows(reader, auth.ActionDelete) {
		t.Fatal("documents:read allowed documents:delete")
	}
	extended := auth.Principal{Subject: "amara", Tenant: "acme", Scopes: auth.NewScopeSet([]string{"documents:readonly"})}
	if policy.Allows(extended, auth.ActionRead) {
		t.Fatal("documents:readonly allowed documents:read")
	}
	if policy.Allows(reader, "documents:frobnicate") {
		t.Fatal("an action the policy does not carry was allowed")
	}
}

func TestExportNeedsBothScopes(t *testing.T) {
	policy := auth.NewPolicy()
	only := auth.Principal{Subject: "amara", Tenant: "acme", Scopes: auth.NewScopeSet([]string{"documents:export"})}
	if policy.Allows(only, auth.ActionExport) {
		t.Fatal("the export action was allowed without documents:read")
	}
	both := auth.Principal{Subject: "amara", Tenant: "acme", Scopes: auth.NewScopeSet([]string{"documents:export", "documents:read"})}
	if !policy.Allows(both, auth.ActionExport) {
		t.Fatal("both scopes did not allow the export")
	}
}

func TestTenantComparisonIsExact(t *testing.T) {
	pr := auth.Principal{Subject: "amara", Tenant: "acme"}
	if !auth.InTenant(pr, "acme") {
		t.Fatal("the same tenant was refused")
	}
	for _, tenant := range []string{"Acme", "ACME", "acme ", "acm"} {
		if auth.InTenant(pr, tenant) {
			t.Fatalf("tenant %q matched acme", tenant)
		}
	}
}

func TestCheckReportsTheMissingScope(t *testing.T) {
	policy := auth.NewPolicy()
	pr := auth.Principal{Subject: "amara", Tenant: "acme", Scopes: auth.NewScopeSet([]string{"documents:read"})}
	err := policy.Check(pr, auth.ActionAdmin)
	if !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("check = %v, want ErrForbidden", err)
	}
	if !strings.Contains(err.Error(), "admin:operate") {
		t.Fatalf("check does not name the missing scope: %v", err)
	}
}
