package access_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/insight/internal/access"
)

const secret = "access-test-secret-0123456789"

func directory(t *testing.T) *access.Directory {
	t.Helper()
	dir := access.NewDirectory()
	for _, a := range []access.Account{
		{Subject: "kelvin", Tenant: "acme", Display: "Kelvin Ortiz", Role: "analyst"},
		{Subject: "amara", Tenant: "acme", Display: "Amara Silva", Role: "admin"},
		{Subject: "bob", Tenant: "globex", Display: "Bob Ceely", Role: "analyst"},
	} {
		if err := dir.Add(a); err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	return dir
}

func issuer(t *testing.T) *access.Issuer {
	t.Helper()
	i, err := access.NewIssuer(secret)
	if err != nil {
		t.Fatalf("issuer: %v", err)
	}
	i.SetClock(func() time.Time { return time.Date(2024, 5, 1, 9, 0, 0, 0, time.UTC) })
	return i
}

func TestFindIsExact(t *testing.T) {
	dir := directory(t)
	if _, ok := dir.Find("kelvin"); !ok {
		t.Fatal("the account was not found")
	}
	if _, ok := dir.Find("Kelvin"); ok {
		t.Fatal("a differently cased subject resolved to the account")
	}
	if _, ok := dir.Find("KELVIN "); ok {
		t.Fatal("a subject with trailing space resolved to the account")
	}
	if !dir.InTenant("kelvin", "acme") {
		t.Fatal("kelvin is not in acme")
	}
	if dir.InTenant("kelvin", "globex") {
		t.Fatal("kelvin resolved in globex")
	}
}

func TestDuplicateSubjectIsRefused(t *testing.T) {
	dir := directory(t)
	if err := dir.Add(access.Account{Subject: "kelvin", Tenant: "globex"}); err == nil {
		t.Fatal("a duplicate subject was accepted")
	}
}

func TestResolverReturnsTheAccountOfTheToken(t *testing.T) {
	dir := directory(t)
	resolver := access.NewResolver(dir)
	pr, err := resolver.Resolve(access.Token{Subject: "amara", Tenant: "acme", Scopes: []string{"reports:read"}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if pr.Subject != "amara" || pr.Tenant != "acme" || pr.Role != "admin" {
		t.Fatalf("principal = %+v", pr)
	}
	if _, err := resolver.Resolve(access.Token{Subject: "nobody", Tenant: "acme"}); !errors.Is(err, access.ErrUnknownSubject) {
		t.Fatalf("unknown subject: err = %v", err)
	}
	if _, err := resolver.Resolve(access.Token{Subject: "kelvin", Tenant: "globex"}); !errors.Is(err, access.ErrUnknownSubject) {
		t.Fatalf("cross-tenant subject: err = %v", err)
	}
}

func TestPolicyIsExactScopeMembership(t *testing.T) {
	policy := access.NewPolicy()
	reader := access.Principal{Subject: "amara", Tenant: "acme", Scopes: access.NewScopes([]string{"reports:read"})}
	if !policy.Allows(reader, access.ActionRead) {
		t.Fatal("reports:read does not allow reports:read")
	}
	extended := access.Principal{Subject: "amara", Tenant: "acme", Scopes: access.NewScopes([]string{"reports:reader"})}
	if policy.Allows(extended, access.ActionRead) {
		t.Fatal("reports:reader allowed reports:read")
	}
	if policy.Allows(reader, access.ActionExport) {
		t.Fatal("the export action was allowed without its scopes")
	}
	if policy.Allows(reader, "reports:frobnicate") {
		t.Fatal("an action the policy does not carry was allowed")
	}
	if err := policy.Check(reader, access.ActionExport); !strings.Contains(err.Error(), "reports:export") {
		t.Fatalf("check = %v", err)
	}
}

func TestTokenRoundTripAndTampering(t *testing.T) {
	i := issuer(t)
	raw := i.Issue("kelvin", "acme", []string{"reports:read"}, time.Hour)
	tok, err := i.Verify(raw)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if tok.Subject != "kelvin" || tok.Tenant != "acme" {
		t.Fatalf("token = %+v", tok)
	}
	if _, err := i.Verify(strings.Replace(raw, "kelvin", "amara", 1)); !errors.Is(err, access.ErrBadSignature) {
		t.Fatalf("tampered token: err = %v", err)
	}
	if _, err := i.Verify(i.Issue("kelvin", "acme", nil, -time.Minute)); !errors.Is(err, access.ErrExpiredToken) {
		t.Fatalf("expired token: err = %v", err)
	}
	if _, err := access.Bearer("Bearer "); err == nil {
		t.Fatal("an empty bearer was accepted")
	}
}
