package relay_test

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"example.com/relay"
)

// recorder records the last request a delivery made and answers with a fixed status.
type recorder struct {
	status int
	host   string
	calls  int
}

func (r *recorder) Do(req *http.Request) (*http.Response, error) {
	r.calls++
	r.host = req.URL.Hostname()
	return &http.Response{
		StatusCode: r.status,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Header:     http.Header{},
	}, nil
}

func open(t *testing.T, rec *recorder) *relay.Service {
	t.Helper()
	service, err := relay.Open(relay.Options{
		Staging:   t.TempDir(),
		Allowlist: []string{"hooks.example.com"},
		Doer:      rec,
		Attempts:  1,
		Backoff:   time.Millisecond,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return service
}

func TestOpenNeedsAnAllowlist(t *testing.T) {
	if _, err := relay.Open(relay.Options{Allowlist: nil, Doer: &recorder{status: 200}, Attempts: 1, Backoff: time.Millisecond}); err != nil {
		t.Fatalf("open without an allowlist: %v", err)
	}
	service := open(t, &recorder{status: 200})
	if service.Guard().Allowlist().Len() != 1 {
		t.Fatalf("allowlist carries %d entries", service.Guard().Allowlist().Len())
	}
}

func TestDeliverToAnAllowedEndpoint(t *testing.T) {
	rec := &recorder{status: 200}
	service := open(t, rec)
	sub := relay.Subscription{
		ID:       "sub-1",
		Tenant:   "acme",
		Endpoint: "https://hooks.example.com/events",
		Secret:   "subscription-secret-0123456789",
		Kinds:    []string{"usage.booked"},
		Active:   true,
	}
	delivery, err := service.Deliver(context.Background(), sub, relay.Event{
		ID: "ev-1", Kind: "usage.booked", Tenant: "acme", Payload: []byte(`{}`), At: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if !delivery.Delivered || delivery.Host != "hooks.example.com" || rec.calls != 1 {
		t.Fatalf("delivery = %+v, calls = %d", delivery, rec.calls)
	}
}

func TestQueueDeliversWhatIsOwed(t *testing.T) {
	rec := &recorder{status: 200}
	service := open(t, rec)
	sub := relay.Subscription{
		ID:       "sub-1",
		Tenant:   "acme",
		Endpoint: "https://hooks.example.com/events",
		Secret:   "subscription-secret-0123456789",
		Kinds:    []string{"usage.booked"},
		Active:   true,
	}
	if _, err := service.Enqueue(sub, relay.Event{ID: "ev-1", Kind: "usage.booked", Tenant: "acme", At: time.Now().UTC()}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if service.Pending() != 1 {
		t.Fatalf("pending = %d", service.Pending())
	}
	results, err := service.RunQueue(context.Background())
	if err != nil {
		t.Fatalf("run queue: %v", err)
	}
	if len(results) != 1 || !results[0].Done || service.Pending() != 0 {
		t.Fatalf("results = %+v, pending = %d", results, service.Pending())
	}
}

func TestEnqueueRefusesAnUnwantedEvent(t *testing.T) {
	service := open(t, &recorder{status: 200})
	sub := relay.Subscription{
		ID:       "sub-1",
		Tenant:   "acme",
		Endpoint: "https://hooks.example.com/events",
		Secret:   "subscription-secret-0123456789",
		Kinds:    []string{"usage.archived"},
		Active:   true,
	}
	if _, err := service.Enqueue(sub, relay.Event{ID: "ev-1", Kind: "usage.booked", Tenant: "acme"}); err == nil {
		t.Fatal("an event the subscription does not ask for was queued")
	}
}

func stream(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	for name, body := range files {
		if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatalf("header: %v", err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("body: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return buf.Bytes()
}

func TestImportStreamAppliesAWholeStream(t *testing.T) {
	service := open(t, &recorder{status: 200})
	report, err := service.ImportStream(bytes.NewReader(stream(t, map[string]string{
		"attachments/logo.png": "png",
		"thumbnails/small.png": "small",
		"records/one.json":     `{"id":"1","kind":"usage","account":"acme","amount":100}`,
		"records/two.json":     `{"id":"2","kind":"usage","account":"acme","amount":250}`,
	})))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if report.Entries != 4 || report.Written != 2 || report.Records != 2 {
		t.Fatalf("report = %+v", report)
	}
	if service.Store().Count() != 2 || service.Store().Total() != 350 {
		t.Fatalf("store: count=%d total=%d", service.Store().Count(), service.Store().Total())
	}
}

func TestImportStreamEnforcesTheStreamLimits(t *testing.T) {
	service := open(t, &recorder{status: 200})
	_, err := service.ImportStreamWithin(bytes.NewReader(stream(t, map[string]string{
		"attachments/logo.png": strings.Repeat("x", 4096),
	})), relay.ArchiveLimits{MaxEntries: 5, MaxEntry: 1024, MaxTotal: 2048})
	if err == nil {
		t.Fatal("a stream above the entry limit was imported")
	}
}
