// Package artifacts wires the artifact server from its options: the tree under the root, the record
// store, the outbound guard and its allowlist, the packaging tools, the token issuer, the trail and the
// HTTP surface. It is the package a deployment and a test both open the server through.
package artifacts

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"example.com/artifacts/internal/artifacts"
	"example.com/artifacts/internal/audit"
	"example.com/artifacts/internal/auth"
	"example.com/artifacts/internal/clock"
	"example.com/artifacts/internal/config"
	"example.com/artifacts/internal/fetch"
	"example.com/artifacts/internal/httpapi"
	"example.com/artifacts/internal/ids"
	"example.com/artifacts/internal/netguard"
	"example.com/artifacts/internal/notify"
	"example.com/artifacts/internal/packaging"
	"example.com/artifacts/internal/storage"
)

// Errors Open reports.
var (
	// ErrOptions is returned for an options value that cannot describe a deployment.
	ErrOptions = errors.New("artifacts: the options are not a usable deployment")
)

// Doer performs one outbound request. A deployment leaves it nil and the server uses an HTTP client; a
// test hands over a recorder, so a delivery to an address that must not be called is observed instead
// of attempted.
type Doer func(req *http.Request) (*http.Response, error)

// Options are the values a deployment runs with.
type Options struct {
	Addr          string
	Root          string
	Projects      []string
	Allowlist     []string
	TokenSecret   string
	Now           time.Time
	Doer          Doer
	PageSize      int
	MaxBody       int64
	MaxArtifact   int64
	RetentionDays int
	KeepLast      int
	// OperatorSubjects are the subjects the deployment names as its operators.
	OperatorSubjects []string
}

// Service is one open artifact server.
type Service struct {
	cfg    config.Config
	root   string
	layout *storage.Layout
	store  *artifacts.Store
	guard  *netguard.Guard
	relay  *notify.Relay
	packer *packaging.Packer
	issuer *auth.Issuer
	policy *auth.Policy
	trail  *audit.Trail
	api    *httpapi.API
	mu     sync.RWMutex
	now    clock.Clock
	subs   map[string][]notify.Subscription
}

// Open validates the options and builds the service.
func Open(opts Options) (*Service, error) {
	cfg := config.Default()
	cfg = cfg.Overlay(config.Config{
		Addr: opts.Addr, Root: opts.Root, Projects: opts.Projects,
		Allowlist: opts.Allowlist, TokenSecret: opts.TokenSecret,
		PageSize: opts.PageSize, MaxBody: opts.MaxBody, MaxArtifact: opts.MaxArtifact,
		Retention: opts.RetentionDays, KeepLast: opts.KeepLast,
	})
	for _, subject := range opts.OperatorSubjects {
		cfg.Tools = append(cfg.Tools, "operator:"+subject)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOptions, err)
	}
	now := clock.System()
	if !opts.Now.IsZero() {
		now = clock.Fixed(opts.Now)
	}
	layout, err := storage.Open(cfg.Root)
	if err != nil {
		return nil, fmt.Errorf("artifacts: %w", err)
	}
	store, err := artifacts.Open(now, cfg.MaxArtifact*int64(cfg.MaxArtifacts))
	if err != nil {
		return nil, fmt.Errorf("artifacts: %w", err)
	}
	guard := netguard.New(netguard.NewAllowlist(cfg.Allowlist), nil)
	client := fetch.New(doerOf(opts.Doer))
	relay := notify.New(guard, client)
	relay.SetRetries(1, time.Millisecond)
	relay.SetSleep(func(time.Duration) {})
	packer := packaging.NewPacker(nil)
	issuer, err := auth.NewIssuer(cfg.TokenSecret, cfg.SessionTTL, now)
	if err != nil {
		return nil, fmt.Errorf("artifacts: %w", err)
	}
	service := &Service{
		cfg: cfg, root: layout.Root(), layout: layout, store: store, guard: guard,
		relay: relay, packer: packer, issuer: issuer, policy: auth.DefaultPolicy(),
		trail: audit.New(1024, now), now: now,
		subs: make(map[string][]notify.Subscription, len(cfg.Projects)),
	}
	api, err := httpapi.New(httpapi.Deps{
		Config: cfg, Store: store, Layout: layout, Packer: packer, Relay: relay,
		Issuer: issuer, Policy: service.policy, Trail: service.trail,
		Subscriptions:   service.subscriptionsOf,
		AddSubscription: service.addSubscription,
		Now:             now,
	})
	if err != nil {
		return nil, fmt.Errorf("artifacts: %w", err)
	}
	service.api = api
	return service, nil
}

// doerOf returns the fetch doer an options value describes.
func doerOf(doer Doer) fetch.Doer {
	if doer == nil {
		return nil
	}
	return doerFunc(doer)
}

// doerFunc adapts an options Doer to the fetch package's interface without exposing the interface.
type doerFunc func(req *http.Request) (*http.Response, error)

// Do performs the request.
func (d doerFunc) Do(req *http.Request) (*http.Response, error) { return d(req) }

// Handler returns the HTTP surface, serving every path the server does not own to next.
func (s *Service) Handler(next http.Handler) http.Handler {
	if next == nil {
		return s.api
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || len(r.URL.Path) > 5 && r.URL.Path[:5] == "/api/" {
			s.api.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SetClock pins every clock the service reads.
func (s *Service) SetClock(at time.Time) {
	fixed := clock.Fixed(at)
	s.mu.Lock()
	s.now = fixed
	s.mu.Unlock()
	s.store.SetClock(fixed)
	s.issuer.SetClock(fixed)
	s.trail.SetClock(fixed)
	s.api.SetClock(fixed)
}

// Issue signs a token for a subject in a project with the scopes given.
func (s *Service) Issue(subject, project string, scopes []string, ttl time.Duration) (string, error) {
	if !s.cfg.HasProject(project) {
		return "", fmt.Errorf("artifacts: this deployment does not serve project %q", project)
	}
	return s.issuer.Issue(subject, project, scopes, ttl)
}

// Verify checks a token.
func (s *Service) Verify(token string) (auth.Claims, error) { return s.issuer.Verify(token) }

// Store returns the record store.
func (s *Service) Store() *artifacts.Store { return s.store }

// Layout returns the tree.
func (s *Service) Layout() *storage.Layout { return s.layout }

// Config returns the validated configuration.
func (s *Service) Config() config.Config { return s.cfg }

// Trail returns the audit trail.
func (s *Service) Trail() *audit.Trail { return s.trail }

// Now returns the service's current instant.
func (s *Service) Now() time.Time {
	s.mu.RLock()
	now := s.now
	s.mu.RUnlock()
	return now().UTC()
}

// subscriptionsOf returns the subscriptions of a project.
func (s *Service) subscriptionsOf(project string) []notify.Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]notify.Subscription, len(s.subs[project]))
	copy(out, s.subs[project])
	return out
}

// addSubscription stores one subscription.
func (s *Service) addSubscription(sub notify.Subscription) error {
	if err := sub.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.subs[sub.Project] {
		if existing.ID == sub.ID {
			return fmt.Errorf("%w: subscription %q", artifacts.ErrDuplicate, sub.ID)
		}
	}
	s.subs[sub.Project] = append(s.subs[sub.Project], sub)
	return nil
}

// Subscribe stores a subscription for a project. It is the API the harness and a test use to record an
// endpoint before an event is delivered.
func (s *Service) Subscribe(project, endpoint, secret string, kinds ...string) (string, error) {
	sub := notify.Subscription{
		ID: ids.New(ids.Subscription), Project: project, Endpoint: endpoint,
		Secret: secret, Kinds: kinds, Active: true,
	}
	if err := s.addSubscription(sub); err != nil {
		return "", err
	}
	return sub.ID, nil
}

// Ingest stages an artifact for a project and returns its identifier. It is the API the harness and a
// test use to place an artifact whose name a trigger needs.
func (s *Service) Ingest(project, name, kind string, body []byte) (string, error) {
	if !s.cfg.HasProject(project) {
		return "", fmt.Errorf("artifacts: this deployment does not serve project %q", project)
	}
	now := s.Now()
	job := artifacts.Job{
		ID: ids.New(ids.Job), Project: project, Kind: "ingest",
		Status: artifacts.JobRunning, Attempts: 1, Created: now, Updated: now,
	}
	if err := s.store.PutJob(job); err != nil {
		return "", err
	}
	blob, err := storage.PutBlob(s.root, body)
	if err != nil {
		return "", err
	}
	staging, err := s.layout.StagingDir(job.ID)
	if err != nil {
		return "", err
	}
	if _, err := storage.Write(staging, name, body, 0o644); err != nil {
		return "", err
	}
	record := artifacts.Record{
		ID: ids.New(ids.Artifact), Project: project, Job: job.ID, Name: name, Kind: kind,
		Digest: blob.Digest, Bytes: blob.Bytes, Status: artifacts.Staged,
		Created: now, Updated: now,
	}
	if err := s.store.Put(record); err != nil {
		return "", err
	}
	job.Artifacts = []string{record.ID}
	job.Status = artifacts.JobSucceeded
	job.Updated = s.Now()
	job.Finished = job.Updated
	if err := s.store.UpdateJob(job); err != nil {
		return "", err
	}
	return record.ID, nil
}

// StagingDirOf returns the staging directory the artifact's job wrote to, which is where a pack of it
// reads from and where a repackage of it writes its tool's working files.
func (s *Service) StagingDirOf(artifactID string) (string, error) {
	record, err := s.store.Get(artifactID)
	if err != nil {
		return "", err
	}
	return s.layout.StagingDir(record.Job)
}

// PackResult is what a pack of one artifact came to.
type PackResult struct {
	Artifact string
	Kind     string
	Archive  string
	Entries  []string
	Bytes    int64
	Tool     string
}

// Repackage rebuilds an artifact's archive through the packaging tool and returns what it produced.
func (s *Service) Repackage(ctx context.Context, artifactID string) (PackResult, error) {
	record, err := s.store.Get(artifactID)
	if err != nil {
		return PackResult{}, err
	}
	staging, err := s.layout.StagingDir(record.Job)
	if err != nil {
		return PackResult{}, err
	}
	packs, err := s.layout.PackDir(record.Job)
	if err != nil {
		return PackResult{}, err
	}
	result, err := s.packer.Pack(ctx, packaging.Request{
		Kind: "repackage", SourceDir: staging, OutputDir: packs, Name: record.Name,
	})
	if err != nil {
		return PackResult{}, err
	}
	return PackResult{
		Artifact: record.ID, Kind: result.Kind, Archive: result.Path,
		Entries: result.Entries, Bytes: result.Bytes, Tool: result.Tool,
	}, nil
}

// BroadcastResult is what a broadcast came to.
type BroadcastResult struct {
	Event     string
	Delivered int
	Skipped   int
	Refused   []BroadcastFailure
}

// BroadcastFailure is one endpoint a broadcast did not reach.
type BroadcastFailure struct {
	Subscription string
	Endpoint     string
	Reason       string
}

// BroadcastEvent sends one event to every subscription of a project that asks for it.
func (s *Service) BroadcastEvent(ctx context.Context, project, kind, subject string) (BroadcastResult, error) {
	event := notify.Event{
		ID: ids.New(ids.Event), Project: project, Kind: kind, Subject: subject, At: clock.Format(s.Now()),
	}
	fanout, err := s.relay.Broadcast(ctx, s.subscriptionsOf(project), event)
	out := BroadcastResult{Event: event.ID, Delivered: fanout.Delivered, Skipped: fanout.Skipped}
	for _, refused := range fanout.Refused {
		out.Refused = append(out.Refused, BroadcastFailure(refused))
	}
	return out, err
}

// DeliveryResult is what one guarded delivery came to.
type DeliveryResult struct {
	Event     string
	Delivered bool
	Status    int
	Host      string
	Err       string
}

// DeliverEvent sends one event to one subscription through the guarded delivery path.
func (s *Service) DeliverEvent(ctx context.Context, subscriptionID, project, kind, subject string) (DeliveryResult, error) {
	var selected notify.Subscription
	found := false
	for _, sub := range s.subscriptionsOf(project) {
		if sub.ID == subscriptionID {
			selected, found = sub, true
			break
		}
	}
	if !found {
		return DeliveryResult{}, fmt.Errorf("artifacts: no subscription %q in project %q", subscriptionID, project)
	}
	event := notify.Event{
		ID: ids.New(ids.Event), Project: project, Kind: kind, Subject: subject, At: clock.Format(s.Now()),
	}
	delivery, err := s.relay.Deliver(ctx, selected, event)
	out := DeliveryResult{
		Event: event.ID, Delivered: delivery.Delivered, Status: delivery.Status, Host: delivery.Host,
	}
	if err != nil {
		out.Err = err.Error()
	}
	return out, err
}
