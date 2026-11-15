// Command triggers exercises the planted defects of this case against whichever module it is run
// from: `sh testdata/triggers/run.sh fixture` reads the fixture, and the same command with `all` or
// `keep-<ID>` reads that variant. The output is what KEY.json records as trigger.actual.
//
// The harness lives outside the fixture's packages, so it opens the server through its exported
// options and drives it through the exported service API; the outbound doer is its own recorder, so a
// delivery to an address that must never be called is observed instead of attempted.
package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	server "example.com/artifacts"
)

// secret is the signing secret the server and the subscriptions run with.
const secret = "trigger-secret-0123456789"

// project is the project the harness drives.
const project = "ci"

// metadataEndpoint is the address a delivery must never be posted to: the cloud metadata service.
const metadataEndpoint = "https://169.254.169.254/latest/meta-data/iam/security-credentials/"

// recorded is the harness's outbound doer: it answers every request and remembers the URL it was asked
// to call, which is how the output says whether a delivery was posted.
type recorded struct {
	status   int
	requests []string
}

// Do records the request and answers it without a network.
func (r *recorded) Do(req *http.Request) (*http.Response, error) {
	r.requests = append(r.requests, req.URL.String())
	return &http.Response{
		StatusCode: r.status,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func main() {
	root, err := os.MkdirTemp("", "artifact-trigger-")
	if err != nil {
		fmt.Println("mkdtemp:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(root)
	doer := &recorded{status: 200}
	service, err := server.Open(server.Options{
		Root:             filepath.Join(root, "tree"),
		Projects:         []string{project},
		Allowlist:        []string{"hooks.example.com"},
		TokenSecret:      secret,
		OperatorSubjects: []string{"root"},
		Now:              time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC),
		Doer:             doer.Do,
	})
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	subscription, err := service.Subscribe(project, metadataEndpoint, secret, "artifact.uploaded")
	if err != nil {
		fmt.Println("subscribe:", err)
		os.Exit(1)
	}

	// D1: the broadcast path fans one event out to every subscription of the project.
	fanout, err := service.BroadcastEvent(context.Background(), project, "artifact.uploaded", "report-01")
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	refused := len(fanout.Refused)
	fmt.Printf("D1 BroadcastEvent(%s) -> delivered=%d refused=%d posted=%v reason=%q\n",
		metadataEndpoint, fanout.Delivered, refused, unique(doer.requests), firstReason(fanout, reason))

	// The guarded delivery path is handed the same endpoint and refuses it.
	doer.requests = nil
	delivery, err := service.DeliverEvent(context.Background(), subscription, project, "artifact.uploaded", "report-01")
	reason = ""
	if err != nil {
		reason = err.Error()
	}
	fmt.Printf("ok DeliverEvent(%s) -> delivered=%t posted=%v reason=%q\n",
		metadataEndpoint, delivery.Delivered, unique(doer.requests), reason)

	// D2: an artifact whose name carries a shell metacharacter is repackaged by the packaging tool.
	name := "report.csv;printf pwned>injected.txt"
	id, err := service.Ingest(project, name, "report", []byte("row,column\n1,2\n"))
	if err != nil {
		fmt.Println("ingest:", err)
		os.Exit(1)
	}
	staging, err := service.StagingDirOf(id)
	if err != nil {
		fmt.Println("staging:", err)
		os.Exit(1)
	}
	result, err := service.Repackage(context.Background(), id)
	injected := exists(filepath.Join(staging, "injected.txt"))
	reason = ""
	if err != nil {
		reason = err.Error()
	}
	fmt.Printf("D2 Repackage(%q) -> ok=%t injected=%t entries=%v reason=%q\n",
		name, err == nil, injected, tarEntries(result.Archive), reason)
}

// firstReason returns the reason of the first refused delivery, or the error the broadcast reported.
func firstReason(fanout server.BroadcastResult, fallback string) string {
	if len(fanout.Refused) > 0 {
		return fanout.Refused[0].Reason
	}
	return fallback
}

// unique returns the URLs recorded, sorted and without repeats.
func unique(urls []string) []string {
	seen := make(map[string]bool, len(urls))
	out := make([]string, 0, len(urls))
	for _, url := range urls {
		if seen[url] {
			continue
		}
		seen[url] = true
		out = append(out, url)
	}
	sort.Strings(out)
	return out
}

// tarEntries returns the entry names of a gzipped tar archive, or nil when the archive carries none.
func tarEntries(path string) []string {
	if path == "" {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return nil
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	var names []string
	for {
		header, err := reader.Next()
		if err != nil {
			return names
		}
		names = append(names, header.Name)
	}
}

// exists reports whether a path exists.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
