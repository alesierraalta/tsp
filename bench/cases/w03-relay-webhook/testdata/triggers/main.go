// Command triggers exercises the planted defects of this case against whichever module it is run
// from: `cd fixture && go run ../testdata/triggers/main.go` reads the fixture, and the same command
// with the run.sh overlay argument reads a variant. The output is what KEY.json records.
package main

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"example.com/relay"
)

// recorder stands in for the HTTP client: it records the host a delivery actually called and always
// answers with 200, so the only thing D1 shows is which host the service decided to call.
type recorder struct {
	host  string
	calls int
}

func (r *recorder) Do(req *http.Request) (*http.Response, error) {
	r.calls++
	r.host = req.URL.Hostname()
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Header:     http.Header{},
	}, nil
}

func main() {
	work, err := os.MkdirTemp("", "relay-trigger-")
	if err != nil {
		fmt.Println("mkdtemp:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(work)

	rec := &recorder{}
	service, err := relay.Open(relay.Options{
		Staging:   filepath.Join(work, "staging"),
		Allowlist: []string{"hooks.example.com"},
		Doer:      rec,
		Attempts:  1,
		Backoff:   time.Millisecond,
	})
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}

	// D1: the endpoint's text begins with the scheme the service trusts, but the URL parses to the
	// cloud metadata address.
	sub := relay.Subscription{
		ID:       "sub-1",
		Tenant:   "acme",
		Endpoint: "https://hooks.example.com@169.254.169.254/latest/meta-data/iam/security-credentials/",
		Secret:   "subscription-secret-0123456789",
		Kinds:    []string{"usage.booked"},
		Active:   true,
	}
	delivery, err := service.Deliver(context.Background(), sub, relay.Event{
		ID: "ev-1", Kind: "usage.booked", Tenant: "acme", Payload: []byte(`{}`),
		At: time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC),
	})
	fmt.Printf("D1 Deliver(endpoint=https://hooks.example.com@169.254.169.254/latest/meta-data/...) -> host=%q status=%d err=%v\n",
		delivery.Host, delivery.Status, err)

	// D2: a thumbnail entry names a path that walks out of the staging directory.
	target := "../../escaped.txt"
	staged, err := service.Stage(relay.Entry{Name: "thumbnails/" + target, Body: []byte("escaped")})
	fmt.Printf("D2 Stage(thumbnails/../../escaped.txt) -> path=%q written_outside=%v err=%v\n",
		relative(work, staged), exists(filepath.Join(work, "escaped.txt")), err)

	// Siblings that do guard the same input, so the run shows the defect is one entry point.
	guarded, err := service.Stage(relay.Entry{Name: "attachments/" + target, Body: []byte("escaped")})
	fmt.Printf("ok Stage(attachments/../../escaped.txt) -> path=%q err=%v\n", relative(work, guarded), err)

	if err := service.CheckEndpoint(sub.Endpoint); err != nil {
		fmt.Printf("ok CheckEndpoint(metadata endpoint) -> err=%v\n", err)
	} else {
		fmt.Println("ok CheckEndpoint(metadata endpoint) -> err=<nil>")
	}
	_ = service.CheckEndpoint(sub.Endpoint)

	// An import with the entry kinds the pipeline understands, to show the happy path still works.
	report, err := service.ImportStream(bytes.NewReader(stream(map[string]string{
		"attachments/logo.png": "png",
		"records/one.json":     `{"id":"1","kind":"usage","account":"acme","amount":100}`,
	})))
	fmt.Printf("ok ImportStream(2 entries) -> written=%d records=%d total=%d err=%v\n",
		report.Written, report.Records, service.Store().Total(), err)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// relative renders a written path against the trigger's working directory, so the output does not
// carry a temporary directory name.
func relative(work, path string) string {
	if path == "" {
		return ""
	}
	rel, err := filepath.Rel(work, path)
	if err != nil {
		return path
	}
	return rel
}

func stream(files map[string]string) []byte {
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	for name, body := range files {
		if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			return nil
		}
		if _, err := w.Write([]byte(body)); err != nil {
			return nil
		}
	}
	if err := w.Close(); err != nil {
		return nil
	}
	return buf.Bytes()
}
