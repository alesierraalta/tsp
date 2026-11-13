package egress_test

import (
	"errors"
	"testing"

	"example.com/shiptrack/internal/egress"
)

func TestHostRuleIncludesTheDot(t *testing.T) {
	guard, err := egress.NewGuard([]string{"example.com"}, false)
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	for _, endpoint := range []string{
		"https://hooks.example.com/deliver",
		"https://example.com/deliver",
		"https://api.eu.example.com/hook?tenant=acme",
	} {
		if _, err := guard.Parse(endpoint); err != nil {
			t.Fatalf("%s was refused: %v", endpoint, err)
		}
	}
	for _, endpoint := range []string{
		"https://notexample.com/deliver",
		"https://example.com.evil.test/deliver",
		"https://example.com@evil.test/deliver",
		"https://example.com./deliver",
	} {
		if _, err := guard.Parse(endpoint); err == nil {
			t.Fatalf("%s was allowed", endpoint)
		}
	}
}

func TestAddressLiteralIsExact(t *testing.T) {
	guard, err := egress.NewGuard([]string{"2.3.4"}, true)
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	// 1.2.3.4 ends with ".2.3.4" as a string; the literal rule is why that is not a match.
	if _, err := guard.Parse("http://1.2.3.4/hook"); err == nil {
		t.Fatal("an address was allowed through a suffix entry")
	}
	strict, err := egress.NewGuard([]string{"127.0.0.1"}, true)
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	if _, err := strict.Parse("http://127.0.0.1:8099/hook"); err != nil {
		t.Fatalf("an exact private host was refused: %v", err)
	}
}

func TestShapeRules(t *testing.T) {
	guard, err := egress.NewGuard([]string{"example.com"}, false)
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	for _, endpoint := range []string{
		"http://example.com/deliver",          // http without an insecure deployment
		"https://user:pw@example.com/deliver", // userinfo
		"https://example.com/deliver#frag",    // fragment
		"https://example.com:8443/deliver",    // a port other than 443
		"https://example.com",                 // no path
		"mailto:ops@example.com",
	} {
		if _, err := guard.Parse(endpoint); err == nil {
			t.Fatalf("%s was allowed", endpoint)
		}
	}
	if _, err := egress.NewGuard([]string{"https://example.com"}, false); err == nil {
		t.Fatal("an entry carrying a scheme was accepted")
	}
	if _, err := egress.NewGuard(nil, false); !errors.Is(err, egress.ErrNoGuard) {
		t.Fatalf("an empty partner list returned %v", err)
	}
}

func TestInsecureHTTPOnlyForExactlyNamedHosts(t *testing.T) {
	guard, err := egress.NewGuard([]string{"example.com", "127.0.0.1"}, true)
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	// A host the list carries exactly may listen on another port in a private network.
	if _, err := guard.Parse("http://127.0.0.1:8099/hook"); err != nil {
		t.Fatalf("exact host on another port: %v", err)
	}
	// A suffix match is allowed plain http on the default port only.
	if _, err := guard.Parse("http://hooks.example.com:8099/hook"); err == nil {
		t.Fatal("a suffix host was allowed on another port")
	}
	if _, err := guard.Parse("http://hooks.example.com/hook"); err != nil {
		t.Fatalf("suffix host on port 80: %v", err)
	}
}
