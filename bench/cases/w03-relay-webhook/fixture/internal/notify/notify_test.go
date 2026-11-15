package notify_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"example.com/relay/internal/netguard"
	"example.com/relay/internal/notify"
)

// recorder is the HTTP client a test uses: it records the request and answers with the status set.
type recorder struct {
	status int
	req    *http.Request
	body   string
	calls  int
}

func (r *recorder) Do(req *http.Request) (*http.Response, error) {
	r.calls++
	r.req = req
	body, _ := io.ReadAll(req.Body)
	r.body = string(body)
	return &http.Response{
		StatusCode: r.status,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Header:     http.Header{},
	}, nil
}

func notifier(t *testing.T) (*notify.Notifier, *recorder) {
	t.Helper()
	rec := &recorder{status: 200}
	n := notify.New(netguard.New([]string{"hooks.example.com"}), rec)
	n.SetSleep(func(time.Duration) {})
	return n, rec
}

func subscription() notify.Subscription {
	return notify.Subscription{
		ID:       "sub-1",
		Tenant:   "acme",
		Endpoint: "https://hooks.example.com/events",
		Secret:   "subscription-secret-0123456789",
		Kinds:    []string{"usage.booked"},
		Active:   true,
	}
}

func event() notify.Event {
	return notify.Event{
		ID:      "ev-1",
		Kind:    "usage.booked",
		Tenant:  "acme",
		Payload: []byte(`{"amount":100}`),
		At:      time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC),
	}
}

func TestDeliverPostsASignedEnvelope(t *testing.T) {
	n, rec := notifier(t)
	delivery, err := n.Deliver(context.Background(), subscription(), event())
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if !delivery.Delivered || delivery.Status != 200 || delivery.Attempts != 1 {
		t.Fatalf("delivery = %+v", delivery)
	}
	if rec.req.URL.Host != "hooks.example.com" {
		t.Fatalf("posted to %q", rec.req.URL.Host)
	}
	signature := rec.req.Header.Get(notify.SignatureHeader)
	if !notify.Verify(subscription().Secret, []byte(rec.body), signature) {
		t.Fatalf("the body was not signed by the subscription: %q", signature)
	}
	if !strings.Contains(rec.body, `"kind":"usage.booked"`) {
		t.Fatalf("body = %s", rec.body)
	}
}

func TestDeliveryRetriesAFailingEndpoint(t *testing.T) {
	n, rec := notifier(t)
	rec.status = 500
	delivery, err := n.Deliver(context.Background(), subscription(), event())
	if !errors.Is(err, notify.ErrFailed) {
		t.Fatalf("deliver: err = %v", err)
	}
	if delivery.Delivered || delivery.Attempts != 3 || rec.calls != 3 {
		t.Fatalf("delivery = %+v, calls = %d", delivery, rec.calls)
	}
}

func TestInactiveSubscriptionIsRefused(t *testing.T) {
	n, rec := notifier(t)
	sub := subscription()
	sub.Active = false
	if _, err := n.Deliver(context.Background(), sub, event()); err == nil {
		t.Fatal("an inactive subscription delivered")
	}
	if rec.calls != 0 {
		t.Fatal("an inactive subscription reached the client")
	}
}

func TestSubscriptionThatDoesNotWantTheEventIsRefused(t *testing.T) {
	n, rec := notifier(t)
	sub := subscription()
	sub.Kinds = []string{"usage.archived"}
	if _, err := n.Deliver(context.Background(), sub, event()); !errors.Is(err, notify.ErrNotWanted) {
		t.Fatalf("deliver: err = %v", err)
	}
	if rec.calls != 0 {
		t.Fatal("an unwanted event reached the client")
	}
}

func TestSubscriptionValidation(t *testing.T) {
	for _, mutate := range []func(*notify.Subscription){
		func(s *notify.Subscription) { s.ID = "" },
		func(s *notify.Subscription) { s.Endpoint = "" },
		func(s *notify.Subscription) { s.Secret = "short" },
	} {
		sub := subscription()
		mutate(&sub)
		if err := sub.Validate(); !errors.Is(err, notify.ErrInvalid) {
			t.Fatalf("subscription %+v: err = %v", sub, err)
		}
	}
}

func TestRegistryKeepsOneSubscriptionPerID(t *testing.T) {
	registry := notify.NewRegistry()
	if err := registry.Add(subscription()); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := registry.Add(subscription()); err == nil {
		t.Fatal("a duplicate subscription id was accepted")
	}
	other := subscription()
	other.ID = "sub-2"
	other.Kinds = []string{"usage.archived"}
	if err := registry.Add(other); err != nil {
		t.Fatalf("add: %v", err)
	}
	if got := registry.Wanting("acme", "usage.booked"); len(got) != 1 || got[0].ID != "sub-1" {
		t.Fatalf("wanting = %+v", got)
	}
	if got := registry.Endpoints(); len(got) != 2 {
		t.Fatalf("endpoints = %v", got)
	}
}
