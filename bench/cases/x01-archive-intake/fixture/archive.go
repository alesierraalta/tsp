// Package archive wires an intake service from its options: the configuration, the staging tree, the
// object store, the pipeline and the HTTP surface. It is the package a deployment and a test both open
// the service through, so the wiring exists once.
package archive

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"example.com/archive/internal/api"
	"example.com/archive/internal/audit"
	"example.com/archive/internal/auth"
	"example.com/archive/internal/clock"
	"example.com/archive/internal/config"
	"example.com/archive/internal/intake"
	"example.com/archive/internal/manifest"
	"example.com/archive/internal/staging"
	"example.com/archive/internal/store"
)

// Options are the values a deployment runs with. A zero value falls back to the configured default, and
// the combination is validated before anything is opened.
type Options struct {
	Root          string
	Addr          string
	Tenants       []string
	PageSize      int
	MaxUpload     int64
	MaxEntries    int
	MaxPages      int
	TokenSecret   string
	SessionTTL    time.Duration
	RetentionDays int
	Zone          string
}

// Service is one open intake service.
type Service struct {
	cfg     config.Config
	store   *store.Store
	staging *staging.Manager
	runner  *intake.Runner
	issuer  *auth.Issuer
	trail   *audit.Trail
	api     *api.API
	now     clock.Clock
}

// Open validates the options and opens the tree, the store and the pipeline. Every failure before the
// first write leaves the filesystem as it was: the directories are created only after validation.
func Open(opts Options) (*Service, error) {
	cfg := config.Default()
	cfg = cfg.Overlay(config.Config{
		Root:          opts.Root,
		Addr:          opts.Addr,
		Tenants:       opts.Tenants,
		PageSize:      opts.PageSize,
		MaxUpload:     opts.MaxUpload,
		MaxEntries:    opts.MaxEntries,
		MaxPages:      opts.MaxPages,
		TokenSecret:   opts.TokenSecret,
		SessionTTL:    opts.SessionTTL,
		RetentionDays: opts.RetentionDays,
		Zone:          opts.Zone,
	})
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	now := clock.System()
	staged, err := staging.Open(cfg.Root, cfg.Limits(), now)
	if err != nil {
		return nil, err
	}
	tree, err := store.Open(cfg.Root, cfg.Limits(), now)
	if err != nil {
		return nil, err
	}
	issuer, err := auth.NewIssuer(cfg.TokenSecret)
	if err != nil {
		return nil, err
	}
	trail := audit.New(audit.DefaultLimit, now)
	runner := intake.NewRunner(cfg.Limits(), staged, tree, now)
	surface := api.New(cfg, tree, runner, staged, issuer, trail, now)
	return &Service{
		cfg:     cfg,
		store:   tree,
		staging: staged,
		runner:  runner,
		issuer:  issuer,
		trail:   trail,
		api:     surface,
		now:     now,
	}, nil
}

// Handler returns the HTTP handler of the service.
func (s *Service) Handler(logger *log.Logger) http.Handler { return s.api.Handler(logger) }

// Routes lists what the service serves.
func (s *Service) Routes() []string { return s.api.Routes() }

// Config is the validated configuration the service runs with.
func (s *Service) Config() config.Config { return s.cfg }

// Root is the absolute root of the archive.
func (s *Service) Root() string { return s.cfg.Root }

// SetClock replaces the clock the responses are stamped with and the token lifetimes are read from.
func (s *Service) SetClock(now func() time.Time) {
	s.now = now
	s.api.SetClock(now)
	s.issuer.SetClock(now)
}

// Issue signs a token for a subject in a tenant. A deployment issues tokens from its identity provider;
// this method is how a test or a script mints one.
func (s *Service) Issue(subject, tenant string, scopes []string, ttl time.Duration) string {
	return s.issuer.Issue(subject, tenant, scopes, ttl)
}

// TrailLen reports how many audit entries the service holds.
func (s *Service) TrailLen() int { return s.trail.Len() }

// Import accepts a batch from bytes: the manifest and one container per entry, exactly as a caller that
// has the delivery in memory would hand it over. It is the call a pre-seed step and a test use.
func (s *Service) Import(m manifest.Manifest, containers map[string][]byte) (intake.Result, error) {
	upload := intake.NewUpload()
	for name, data := range containers {
		upload.AddContainer(name, data)
	}
	return s.runner.Accept(context.Background(), m, upload)
}

// ImportWithPages accepts a batch that carries page images, which is the shape a scanned batch has.
func (s *Service) ImportWithPages(m manifest.Manifest, containers map[string][]byte, pages map[string]map[int][]byte) (intake.Result, error) {
	upload := intake.NewUpload()
	for name, data := range containers {
		upload.AddContainer(name, data)
	}
	for name, byNumber := range pages {
		for number, data := range byNumber {
			upload.AddPage(name, number, data)
		}
	}
	return s.runner.Accept(context.Background(), m, upload)
}

// ImportManifest accepts a batch from the manifest's JSON bytes and the payload maps a delivery
// carries. It is the call a client that already holds the document in memory uses, and it validates the
// manifest exactly like the multipart endpoint does.
func (s *Service) ImportManifest(raw []byte, containers map[string][]byte, pages map[string]map[int][]byte) (intake.Result, error) {
	m, err := manifest.Decode(raw, s.cfg.Limits())
	if err != nil {
		return intake.Result{}, err
	}
	return s.ImportWithPages(m, containers, pages)
}

// Resume continues an interrupted batch from its staging tree. A batch that committed is reported as
// committed; a batch whose tree is gone is an error.
func (s *Service) Resume(batch string) (intake.Result, error) {
	return s.runner.Resume(context.Background(), batch)
}

// BatchState reports what a staged batch still holds.
func (s *Service) BatchState(batch string) (intake.ResumeState, error) { return s.runner.State(batch) }

// Abort discards a staged batch's tree.
func (s *Service) Abort(batch string) error { return s.runner.Abort(batch) }

// Documents lists the objects one tenant carries, newest first.
func (s *Service) Documents(tenant string, limit int) []store.Record {
	records, err := s.store.List(store.Filter{Tenant: tenant, Limit: limit})
	if err != nil {
		return nil
	}
	return records
}

// Document returns one object's metadata.
func (s *Service) Document(id string) (store.Record, error) { return s.store.Get(id) }

// DocumentText returns the text the pipeline extracted for one object.
func (s *Service) DocumentText(id string) (string, error) { return s.store.ReadText(id) }

// Stats is the store's summary.
func (s *Service) Stats() store.Stats { return s.store.Stats() }

// PDF builds a minimal container for a scan the way the scanner's container writer does: a header line
// with the entry name and page count, then the page payloads. It is a helper for callers that deliver a
// scan without a real PDF, and the pipeline treats its bytes like any other container.
func PDF(name string, pages []string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "%%ARCHIVE-SCAN %s pages=%d\n", name, len(pages))
	for i, page := range pages {
		fmt.Fprintf(&b, "%%PAGE %03d %s\n", i+1, page)
	}
	return b.Bytes()
}

// PGMPage builds one page image the way the scanner writes it: a netpbm header and the recognised text
// as the payload. The pipeline reads the payload and ignores the header.
func PGMPage(text string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "P5\n1 1\n255\n%s\n", text)
	return b.Bytes()
}
