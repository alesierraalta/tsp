// Package relay wires the service: the outbound policy, the notifier, the queue, the import
// pipeline and the record store.
package relay

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"example.com/relay/internal/archive"
	"example.com/relay/internal/fetch"
	"example.com/relay/internal/importer"
	"example.com/relay/internal/jobs"
	"example.com/relay/internal/netguard"
	"example.com/relay/internal/notify"
	"example.com/relay/internal/store"
	"example.com/relay/internal/wire"
)

// Aliases so a caller of this package names the values it works with without reaching into the
// packages that implement them.
type (
	Subscription = notify.Subscription
	Event        = notify.Event
	Delivery     = notify.Delivery
	Entry        = archive.Entry
	ImportReport = importer.Report
	Record       = wire.Record
	// ArchiveLimits is the bound a stream is read under.
	ArchiveLimits = archive.Limits
)

// Options are the values a deployment runs with.
type Options struct {
	Staging   string
	Allowlist []string
	// Doer is the HTTP client deliveries use. A deployment leaves it nil for the bounded default
	// client; a test supplies its own, which is how the outbound decision is observed.
	Doer     fetch.Doer
	Attempts int
	Backoff  time.Duration
}

// Service is one open relay.
type Service struct {
	staging  string
	guard    *netguard.Guard
	notifier *notify.Notifier
	runner   *jobs.Runner
	importer *importer.Importer
	store    *store.Store
}

// Open builds a service from its options.
func Open(opts Options) (*Service, error) {
	guard := netguard.New(opts.Allowlist)
	st := store.New()
	notifier := notify.New(guard, opts.Doer)
	if opts.Attempts > 0 || opts.Backoff > 0 {
		notifier.SetRetries(opts.Attempts, opts.Backoff)
	}
	staging := opts.Staging
	if staging == "" {
		created, err := os.MkdirTemp("", "relay-staging-")
		if err != nil {
			return nil, fmt.Errorf("relay: staging directory: %w", err)
		}
		staging = created
	}
	return &Service{
		staging:  staging,
		guard:    guard,
		notifier: notifier,
		runner:   jobs.New(notifier),
		importer: importer.New(staging, st),
		store:    st,
	}, nil
}

// Guard is the outbound policy of the instance.
func (s *Service) Guard() *netguard.Guard { return s.guard }

// Staging is the directory the import pipeline writes to.
func (s *Service) Staging() string { return s.staging }

// Store is the record store of the instance.
func (s *Service) Store() *store.Store { return s.store }

// CheckEndpoint reports whether the guard would allow a delivery to endpoint.
func (s *Service) CheckEndpoint(endpoint string) error { return s.guard.CheckURL(endpoint) }

// Enqueue queues one delivery.
func (s *Service) Enqueue(sub Subscription, ev Event) (string, error) {
	return s.runner.Enqueue(sub, ev)
}

// Deliver delivers one event immediately, without the queue.
func (s *Service) Deliver(ctx context.Context, sub Subscription, ev Event) (Delivery, error) {
	return s.notifier.Deliver(ctx, sub, ev)
}

// RunQueue delivers every pending job.
func (s *Service) RunQueue(ctx context.Context) ([]jobs.Job, error) { return s.runner.Run(ctx) }

// Pending reports how many jobs are owed.
func (s *Service) Pending() int { return s.runner.Pending() }

// ImportStream reads a tar stream under the default limits and applies it.
func (s *Service) ImportStream(r io.Reader) (ImportReport, error) {
	return s.ImportStreamWithin(r, archive.DefaultLimits())
}

// ImportStreamWithin reads a tar stream under the limits given and applies it.
func (s *Service) ImportStreamWithin(r io.Reader, limits archive.Limits) (ImportReport, error) {
	entries, err := archive.Read(r, limits)
	if err != nil {
		return ImportReport{}, err
	}
	return s.importer.Run(entries)
}

// Stage writes one entry through the staging rule of its kind.
func (s *Service) Stage(entry Entry) (string, error) {
	switch kind := archive.Kind(entry.Name); kind {
	case importer.KindAttachments:
		return importer.StageAttachment(s.staging, entry)
	case importer.KindThumbnails:
		return importer.StageThumbnail(s.staging, entry)
	default:
		return "", fmt.Errorf("relay: %q is under %q, which is not a staging rule", entry.Name, kind)
	}
}
