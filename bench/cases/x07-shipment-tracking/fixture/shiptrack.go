// Package shiptrack wires a shipment-tracking deployment from its configuration: the workspace, the
// partner and carrier directory, the rate book, the store, the trail, the token issuer, the label
// renderer, the notification client and the HTTP surface. It is the package a deployment and a test both
// open the service through, so the wiring exists once and a test exercises what production runs.
package shiptrack

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"example.com/shiptrack/internal/api"
	"example.com/shiptrack/internal/audit"
	"example.com/shiptrack/internal/auth"
	"example.com/shiptrack/internal/clock"
	"example.com/shiptrack/internal/config"
	"example.com/shiptrack/internal/egress"
	"example.com/shiptrack/internal/identity"
	"example.com/shiptrack/internal/labels"
	"example.com/shiptrack/internal/notify"
	"example.com/shiptrack/internal/statement"
	"example.com/shiptrack/internal/store"
	"example.com/shiptrack/internal/tariff"
	"example.com/shiptrack/internal/workspace"
)

// ErrConfig is reported for a configuration the service cannot open.
var ErrConfig = errors.New("shiptrack: configuration refused")

// Service is one open deployment.
type Service struct {
	cfg       *config.Config
	workspace *workspace.Workspace
	directory *identity.Directory
	book      *tariff.Book
	store     *store.Store
	trail     *audit.Trail
	issuer    *auth.Issuer
	sessions  *auth.Sessions
	notify    *notify.Queue
	renderer  *labels.Runner
	clock     *clock.Clock
	api       *api.API
}

// Open validates a configuration and wires the service. Nothing is written to the root until a request
// writes it: a refused configuration leaves the filesystem as it was.
func Open(cfg *config.Config) (*Service, error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w: no configuration", ErrConfig)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConfig, err)
	}
	tree, err := workspace.Open(cfg.Root)
	if err != nil {
		return nil, err
	}
	book, err := tariff.NewBook(cfg.TaxRate)
	if err != nil {
		return nil, err
	}
	for _, service := range tariff.DefaultServices() {
		if _, err := book.AddService(service); err != nil {
			return nil, err
		}
	}
	records, err := store.New(cfg.Caps.MaxEvents, cfg.Caps.MaxShipments)
	if err != nil {
		return nil, err
	}
	trail, err := audit.New(cfg.Caps.TrailCap)
	if err != nil {
		return nil, err
	}
	issuer, err := auth.NewIssuer(cfg.TokenSecret, cfg.TokenTTL)
	if err != nil {
		return nil, err
	}
	sessions, err := auth.NewSessions(cfg.TokenTTL, 10000)
	if err != nil {
		return nil, err
	}
	var queue *notify.Queue
	if len(cfg.EgressHosts) > 0 {
		guard, err := egress.NewGuard(cfg.EgressHosts, cfg.Insecure)
		if err != nil {
			return nil, err
		}
		client, err := egress.NewClient(guard, cfg.Caps.MaxResponse, 10*time.Second)
		if err != nil {
			return nil, err
		}
		if queue, err = notify.New(client, 2); err != nil {
			return nil, err
		}
	}
	var renderer *labels.Runner
	if cfg.Renderer != "" {
		if renderer, err = labels.New(cfg.Renderer, 1<<20, 32<<20); err != nil {
			return nil, err
		}
	}
	service := &Service{
		cfg:       cfg,
		workspace: tree,
		directory: identity.NewDirectory(),
		book:      book,
		store:     records,
		trail:     trail,
		issuer:    issuer,
		sessions:  sessions,
		notify:    queue,
		renderer:  renderer,
		clock:     clock.System(),
	}
	surface, err := api.New(api.Deps{
		Config:    cfg,
		Directory: service.directory,
		Book:      book,
		Store:     records,
		Trail:     trail,
		Workspace: tree,
		Renderer:  renderer,
		Notify:    queue,
		Issuer:    issuer,
		Sessions:  sessions,
		Clock:     service.clock,
	})
	if err != nil {
		return nil, err
	}
	service.api = surface
	return service, nil
}

// Handler returns the surface's HTTP handler.
func (s *Service) Handler(logger *log.Logger) http.Handler { return s.api.Handler(logger) }

// Routes lists what the service serves.
func (s *Service) Routes() []string { return s.api.Routes() }

// Config is the configuration the service opened with.
func (s *Service) Config() *config.Config { return s.cfg }

// Root is the absolute workspace root.
func (s *Service) Root() string { return s.workspace.Root() }

// TrailLen reports how many audit entries the service carries.
func (s *Service) TrailLen() int { return s.trail.Len() }

// Workspace is the tree the service reads and writes through.
func (s *Service) Workspace() *workspace.Workspace { return s.workspace }

// Store is the service's record of what it ingested and closed.
func (s *Service) Store() *store.Store { return s.store }

// Book is the rate book.
func (s *Service) Book() *tariff.Book { return s.book }

// Directory is the partner and carrier directory.
func (s *Service) Directory() *identity.Directory { return s.directory }

// Trail is the audit trail.
func (s *Service) Trail() *audit.Trail { return s.trail }

// Queue is the notification queue, or nil when the deployment lists no partner hosts.
func (s *Service) Queue() *notify.Queue { return s.notify }

// Renderer is the label renderer, or nil when the deployment configures none.
func (s *Service) Renderer() *labels.Runner { return s.renderer }

// SetClock replaces the clock the surface reads, so a test can move a close past a month boundary
// without waiting for one.
func (s *Service) SetClock(now func() time.Time) {
	if now == nil {
		return
	}
	s.clock.SetNow(now)
	s.api.SetClock(now)
}

// Now is the service's current instant in UTC.
func (s *Service) Now() time.Time { return s.clock.Now() }

// Issue signs a token for a subject, a tenant and its scopes. A deployment mints tokens from its identity
// provider; this method is how a test or a script mints one.
func (s *Service) Issue(subject, tenant string, scopes []string) string {
	return s.issuer.Issue(subject, tenant, scopes)
}

// IssueAt signs a token as if it were issued at an instant, for a test of an expired one.
func (s *Service) IssueAt(subject, tenant string, scopes []string, at time.Time) string {
	return s.issuer.IssueAt(subject, tenant, scopes, at)
}

// RegisterPartner adds a partner. The tenant must be one the deployment serves, and the partner's id and
// endpoint are validated by the directory and the egress guard, so a registration cannot add a partner a
// request could name another tenant's endpoint through.
func (s *Service) RegisterPartner(partner identity.Partner) (identity.Partner, error) {
	if !s.cfg.TenantAllowed(partner.Tenant) {
		return identity.Partner{}, fmt.Errorf("%w: tenant %q", identity.ErrTenant, partner.Tenant)
	}
	return s.directory.AddPartner(partner)
}

// RegisterCarrier adds a carrier with its lanes.
func (s *Service) RegisterCarrier(carrier identity.Carrier) (identity.Carrier, error) {
	return s.directory.AddCarrier(carrier)
}

// RegisterService adds a rate to the book.
func (s *Service) RegisterService(service tariff.Service) (tariff.Service, error) {
	return s.book.AddService(service)
}

// ClosePeriod closes a partner's statement outside the HTTP surface, which is what an operator's script
// does after a month ends. It reads the same period rules, the same rate book and the same statement
// assembly the route does: there is one close, not two.
func (s *Service) ClosePeriod(ctx context.Context, partner identity.Partner, period clock.Period) (statement.Statement, error) {
	now := s.Now()
	if err := statement.CheckCloseable(period, now, s.cfg.Caps.CloseWindowMonths); err != nil {
		return statement.Statement{}, err
	}
	items := make([]statement.Item, 0, 64)
	for _, shipment := range s.store.Shipments(partner.Tenant, partner.ID) {
		if !period.Contains(shipment.Created) {
			continue
		}
		quote, err := s.book.Rate(shipment.Service, shipment.Grams)
		if err != nil {
			return statement.Statement{}, err
		}
		items = append(items, statement.Item{
			Shipment: shipment.ID,
			Carrier:  shipment.Carrier,
			Service:  shipment.Service,
			Lane:     shipment.Lane,
			Grams:    shipment.Grams,
			Base:     quote.Base,
			Fuel:     quote.Fuel,
			At:       shipment.Created,
		})
	}
	if len(items) == 0 {
		return statement.Statement{}, fmt.Errorf("%w: partner %q has no shipments in %s", statement.ErrInvalid, partner.ID, period)
	}
	return statement.Build(partner, period, items, s.book.TaxBasisPoints(), now)
}

// Priced reports what the rate book charges for a weight on a service, which is what a partner's console
// shows before it books a shipment.
func (s *Service) Priced(service string, grams int64) (tariff.Quote, error) {
	return s.book.Rate(service, grams)
}
