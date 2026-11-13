package notify

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func guard(t *testing.T, hosts ...string) *Guard {
	t.Helper()
	list, err := NewGuard(hosts)
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	return list
}

func TestCheckDecidesOnTheParsedURL(t *testing.T) {
	list := guard(t, "example.com")

	allowed := []string{
		"https://example.com/hooks",
		"https://hooks.example.com/hooks",
		"https://deep.hooks.example.com:443/hooks",
	}
	for _, endpoint := range allowed {
		if _, err := list.Check(endpoint, true); err != nil {
			t.Fatalf("Check(%q): %v", endpoint, err)
		}
	}

	refused := []string{
		"https://notexample.com/hooks",        // a name that ends with the allowed host without the dot
		"https://example.com.evil.test/hooks", // the allowed host as a prefix
		"https://example.com@evil.test/hooks", // userinfo makes evil.test the host
		"http://example.com/hooks",            // not https
		"https://example.com:8443/hooks",      // a port that is not 443
		"https://example.com/hooks#fragment",
		"https://example.com",
		"ftp://example.com/hooks",
		"",
	}
	for _, endpoint := range refused {
		if _, err := list.Check(endpoint, true); err == nil {
			t.Fatalf("Check(%q) was accepted", endpoint)
		}
	}
}

func TestCheckTreatsAnAddressLiteralExactly(t *testing.T) {
	list := guard(t, "2.3.4")
	if list.Allow("1.2.3.4") {
		t.Fatal("1.2.3.4 was allowed by an entry for 2.3.4")
	}
	literal := guard(t, "127.0.0.1")
	if _, err := literal.Check("http://127.0.0.1:8080/hooks", false); err != nil {
		t.Fatalf("an allowlisted literal over http was refused: %v", err)
	}
	if _, err := literal.Check("http://127.0.0.1:8080/hooks", true); err == nil {
		t.Fatal("a plain-http endpoint was accepted with the TLS floor on")
	}
}

func TestNewGuardRefusesAnEntryItCannotUse(t *testing.T) {
	for _, entry := range []string{"https://example.com", "example.com/hooks", "example.com:443", ""} {
		if _, err := NewGuard([]string{entry}); err == nil {
			t.Fatalf("the partner entry %q was accepted", entry)
		}
	}
}

func TestDeliverPostsAndReadsABoundedResponse(t *testing.T) {
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client, err := NewClient(guard(t, "127.0.0.1"), nil, 4096, false)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	result, err := client.Deliver(context.Background(), server.URL+"/hooks", []byte(`{"invoice":"inv-acme-team-api-2024-05"}`))
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if !result.Delivered || result.Status != http.StatusOK {
		t.Fatalf("result = %+v", result)
	}
	if kind := <-received; kind != "application/json" {
		t.Fatalf("content type %q", kind)
	}
}

func TestDeliverRefusesARedirectAndAnOversizedAnswer(t *testing.T) {
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://evil.test/hooks", http.StatusFound)
	}))
	defer redirect.Close()
	client, err := NewClient(guard(t, "127.0.0.1"), nil, 4096, false)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	if _, err := client.Deliver(context.Background(), redirect.URL+"/hooks", []byte(`{}`)); !errors.Is(err, ErrRedirected) {
		t.Fatalf("a redirect was followed: %v", err)
	}

	large := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("x", 9000)))
	}))
	defer large.Close()
	small, err := NewClient(guard(t, "127.0.0.1"), nil, 1024, false)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	if _, err := small.Deliver(context.Background(), large.URL+"/hooks", []byte(`{}`)); !errors.Is(err, ErrResponse) {
		t.Fatalf("an oversized answer was read: %v", err)
	}
}

func TestDeliverRefusesAnEmptyOrOversizedPayload(t *testing.T) {
	client, err := NewClient(guard(t, "127.0.0.1"), nil, 4096, false)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	if _, err := client.Deliver(context.Background(), "http://127.0.0.1/hooks", nil); !errors.Is(err, ErrPayload) {
		t.Fatalf("an empty payload was delivered: %v", err)
	}
	if _, err := client.Deliver(context.Background(), "http://127.0.0.1/hooks", make([]byte, MaxPayloadBytes+1)); !errors.Is(err, ErrPayload) {
		t.Fatalf("an oversized payload was delivered: %v", err)
	}
}
