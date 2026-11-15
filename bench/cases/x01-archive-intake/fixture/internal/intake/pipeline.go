package intake

import (
	"context"
	"example.com/archive/internal/clock"
	"example.com/archive/internal/config"
	"example.com/archive/internal/manifest"
	"example.com/archive/internal/staging"
	"example.com/archive/internal/store"
	"fmt"
	"sync"
)

// Package intake is the import pipeline: it accepts a batch, verifies what came with it, extracts the
// text of every page, and commits the objects the store keeps. Every stage runs over the staging tree
// and reads the manifest it was validated with, so a stage never repeats a check the accept step made.

// Upload is what a delivery carries besides the manifest: the container of each entry, the page
// images the scanner wrote, and any text sidecar the scanner produced itself. Names are entry names,
// never paths.
type Upload struct {
	Containers map[string][]byte
	Pages      map[string]map[int][]byte
	Sidecars   map[string][]byte
	Order      []string
}

// NewUpload is an empty upload a caller fills in.
func NewUpload() Upload {
	return Upload{Containers: map[string][]byte{}, Pages: map[string]map[int][]byte{}, Sidecars: map[string][]byte{}}
}

// AddContainer records the container bytes of one entry.
func (u *Upload) AddContainer(name string, data []byte) {
	if u.Containers == nil {
		u.Containers = map[string][]byte{}
	}
	u.Containers[name] = data
	u.Order = append(u.Order, name)
}

// AddPage records one page image of one entry.
func (u *Upload) AddPage(name string, page int, data []byte) {
	if u.Pages == nil {
		u.Pages = map[string]map[int][]byte{}
	}
	if u.Pages[name] == nil {
		u.Pages[name] = map[int][]byte{}
	}
	u.Pages[name][page] = data
}

// AddSidecar records a text sidecar the scanner wrote itself.
func (u *Upload) AddSidecar(name string, data []byte) {
	if u.Sidecars == nil {
		u.Sidecars = map[string][]byte{}
	}
	u.Sidecars[name] = data
}

// EntryResult is what the pipeline reports about one entry.
type EntryResult struct {
	Name   string `json:"name"`
	Object string `json:"object,omitempty"`
	Kind   string `json:"kind"`
	Title  string `json:"title,omitempty"`
	Pages  int    `json:"pages"`
	Bytes  int64  `json:"bytes"`
	Digest string `json:"digest,omitempty"`
	Text   string `json:"text,omitempty"`
	Warn   string `json:"warning,omitempty"`
	Err    string `json:"error,omitempty"`
}

// Result is what one accept or resume reports about a batch.
type Result struct {
	Batch     string        `json:"batch"`
	Tenant    string        `json:"tenant"`
	Entries   []EntryResult `json:"entries"`
	Pages     int           `json:"pages"`
	Bytes     int64         `json:"bytes"`
	Committed bool          `json:"committed"`
	Started   string        `json:"started"`
	Finished  string        `json:"finished"`
}

// Failed reports how many entries carry an error.
func (r Result) Failed() int {
	failed := 0
	for _, e := range r.Entries {
		if e.Err != "" {
			failed++
		}
	}
	return failed
}

// Text returns the extracted text of one entry, for a caller that wants the one it asked about.
func (r Result) Text(name string) string {
	for _, e := range r.Entries {
		if e.Name == name {
			return e.Text
		}
	}
	return ""
}

// Runner runs the pipeline for one deployment.
type Runner struct {
	lim     config.Limits
	staging *staging.Manager
	store   *store.Store
	now     clock.Clock
	mu      sync.Mutex
	active  map[string]bool
}

// NewRunner wires a runner over one staging tree and one store.
func NewRunner(lim config.Limits, st *staging.Manager, s *store.Store, now clock.Clock) *Runner {
	return &Runner{lim: lim.Or(), staging: st, store: s, now: now, active: map[string]bool{}}
}

// Staging is the tree the runner stages batches in.
func (r *Runner) Staging() *staging.Manager { return r.staging }

// Accept stages a batch and runs the pipeline over it. The batch goes through the same three stages
// a resume does, so an interrupted batch and a fresh one commit the same objects.
func (r *Runner) Accept(ctx context.Context, m manifest.Manifest, upload Upload) (Result, error) {
	if err := m.Validate(r.lim); err != nil {
		return Result{}, err
	}
	if m.ID == "" {
		m.ID = r.staging.NewBatchID()
	}
	layout, err := r.staging.Layout(m.ID)
	if err != nil {
		return Result{}, err
	}
	if layout.Exists() {
		return Result{}, fmt.Errorf("%w: %s", staging.ErrExists, m.ID)
	}
	if err := layout.WriteManifest(m); err != nil {
		return Result{}, err
	}
	if err := r.stageUpload(layout, m, upload); err != nil {
		return Result{}, err
	}
	return r.run(ctx, layout, m)
}
