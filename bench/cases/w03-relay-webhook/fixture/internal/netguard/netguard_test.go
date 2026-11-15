package netguard_test

import (
	"errors"
	"testing"

	"example.com/relay/internal/netguard"
)

func guard() *netguard.Guard {
	return netguard.New([]string{"hooks.example.com", "partner.example.net"})
}

func TestCheckAllowsAnAllowlistedEndpoint(t *testing.T) {
	target, err := guard().Check("https://hooks.example.com/events")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if target.Host != "hooks.example.com" {
		t.Fatalf("host = %q", target.Host)
	}
	if target.URL != "https://hooks.example.com/events" {
		t.Fatalf("url = %q", target.URL)
	}
}

func TestCheckRefusesEveryShapeOfBadEndpoint(t *testing.T) {
	cases := []struct{ endpoint, why string }{
		{"http://hooks.example.com/events", "plain http"},
		{"https://hooks.example.com.evil.test/events", "a host that extends the allowed one"},
		{"https://hooks.example.com:8443/events", "a port other than 443"},
		{"https://hooks.example.com@169.254.169.254/latest/meta-data/", "userinfo in front of a metadata address"},
		{"https://169.254.169.254/latest/meta-data/", "a metadata address"},
		{"https://10.1.2.3/internal", "a private address"},
		{"https://unlisted.example.com/events", "a host that is not on the list"},
		{"", "an empty endpoint"},
	}
	for _, c := range cases {
		if _, err := guard().Check(c.endpoint); !errors.Is(err, netguard.ErrBlocked) && !errors.Is(err, netguard.ErrMalformed) {
			t.Fatalf("%s (%q): err = %v", c.why, c.endpoint, err)
		}
	}
}

func TestAllowlistIsExactAndCaseInsensitive(t *testing.T) {
	list := netguard.NewAllowlist([]string{"Hooks.Example.com", "https://bad.example.com", "with/path"})
	if !list.Has("hooks.example.com") {
		t.Fatal("a name stored in another case was not found")
	}
	if list.Has("hooks.example.com.evil.test") {
		t.Fatal("a name that extends an entry was found")
	}
	if list.Len() != 1 {
		t.Fatalf("allowlist keeps %d entries, want only the one valid name", list.Len())
	}
}

func TestClassify(t *testing.T) {
	cases := map[string]netguard.Classification{
		"8.8.8.8":           netguard.Public,
		"127.0.0.1":         netguard.Loopback,
		"10.2.3.4":          netguard.Private,
		"192.168.1.10":      netguard.Private,
		"169.254.169.254":   netguard.Metadata,
		"hooks.example.com": netguard.Other,
	}
	for host, want := range cases {
		if got := netguard.Classify(host); got != want {
			t.Fatalf("classify(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestParseHostStripsPortAndBrackets(t *testing.T) {
	for _, raw := range []string{"hooks.example.com:443", "[::1]:443", "HOOKS.example.com."} {
		host, err := netguard.ParseHost(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		if host == "" {
			t.Fatalf("parse %q returned an empty host", raw)
		}
	}
	if _, err := netguard.ParseHost("  "); err == nil {
		t.Fatal("an empty host was accepted")
	}
}

func TestDescribeNamesThePolicy(t *testing.T) {
	policy := netguard.Describe(guard())
	allow, ok := policy["allowlist"].([]string)
	if !ok || len(allow) != 2 {
		t.Fatalf("policy = %+v", policy)
	}
	if ranges, ok := policy["refused_ranges"].([]string); !ok || len(ranges) != 4 {
		t.Fatalf("policy = %+v", policy)
	}
}
