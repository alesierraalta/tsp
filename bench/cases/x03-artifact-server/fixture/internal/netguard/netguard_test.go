package netguard

import (
	"errors"
	"net"
	"testing"
)

// resolver answers every name with the address given.
func resolver(ip string) Resolver {
	return func(host string) ([]net.IP, error) { return []net.IP{net.ParseIP(ip)}, nil }
}

func TestGuardAllowsAnAllowlistedPublicHost(t *testing.T) {
	guard := New(NewAllowlist([]string{"hooks.example.com"}), resolver("93.184.216.34"))
	parsed, err := guard.Check("https://hooks.example.com/deliver?event=1")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if parsed.Hostname() != "hooks.example.com" {
		t.Fatalf("host = %q", parsed.Hostname())
	}
}

func TestGuardRefusesWhatItCannotAllow(t *testing.T) {
	cases := []struct {
		endpoint string
		why      string
	}{
		{"http://hooks.example.com/deliver", "scheme"},
		{"https://hooks.example.com@169.254.169.254/latest/meta-data/", "userinfo"},
		{"https://169.254.169.254/latest/meta-data/", "private"},
		{"https://127.0.0.1/deliver", "loopback"},
		{"https://evil.example.com/deliver", "allowlist"},
		{"https://hooks.example.com:8443/deliver", "port"},
		{"", "empty"},
	}
	for _, c := range cases {
		guard := New(NewAllowlist([]string{"hooks.example.com"}), resolver("93.184.216.34"))
		if _, err := guard.Check(c.endpoint); !errors.Is(err, ErrBlocked) && !errors.Is(err, ErrParse) {
			t.Errorf("%s (%s) was accepted: %v", c.endpoint, c.why, err)
		}
	}
}

func TestGuardRefusesANameThatResolvesIntoTheCluster(t *testing.T) {
	guard := New(NewAllowlist([]string{"internal.example.com"}), resolver("10.0.0.7"))
	if _, err := guard.Check("https://internal.example.com/deliver"); !errors.Is(err, ErrBlocked) {
		t.Fatalf("a name resolving to a private address was accepted: %v", err)
	}
}

func TestAllowlistNormalisesNames(t *testing.T) {
	list := NewAllowlist([]string{"Hooks.Example.com.", "bad/entry", ""})
	if list.Len() != 1 {
		t.Fatalf("len = %d, entries = %v", list.Len(), list.Entries())
	}
	if !list.Has("hooks.example.com") || list.Has("evil.example.com") {
		t.Fatalf("membership is wrong: %v", list.Entries())
	}
	if got := list.With("deploy.example.com").Len(); got != 2 {
		t.Fatalf("with: len = %d", got)
	}
}

func TestPublicAddresses(t *testing.T) {
	if !Public(net.ParseIP("93.184.216.34")) {
		t.Fatal("a public address was refused")
	}
	for _, address := range []string{"127.0.0.1", "10.1.2.3", "169.254.169.254", "::1", "fe80::1"} {
		if Public(net.ParseIP(address)) {
			t.Errorf("%s was accepted as public", address)
		}
	}
	if !IsMetadata(net.ParseIP("169.254.169.254")) {
		t.Fatal("the metadata address was not recognised")
	}
}
