// Package ledgerclose wires a billing close service from its configuration: the statement
// workspace, the account directory, the rate book, the store, the journal and the HTTP surface. It
// is the package a deployment and a test both open the service through, so the wiring exists once.
package ledgerclose

import (
	"log"
	"net/http"
	"time"

	"example.com/ledgerclose/internal/api"
	"example.com/ledgerclose/internal/auth"
	"example.com/ledgerclose/internal/config"
	"example.com/ledgerclose/internal/identity"
	"example.com/ledgerclose/internal/journal"
	"example.com/ledgerclose/internal/notify"
	"example.com/ledgerclose/internal/pdfgen"
	"example.com/ledgerclose/internal/pricing"
	"example.com/ledgerclose/internal/store"
	"example.com/ledgerclose/internal/workspace"
)

// Service is one open close service.
type Service struct {
	cfg       config.Config
	workspace *workspace.Workspace
	directory *identity.Directory
	book      *pricing.Book
	store     *store.Store
	trail     *journal.Journal
	issuer    *auth.Issuer
	api       *api.API
}

// Open validates the configuration and wires the service. Nothing is created in the workspace until
// a close runs; a failure here leaves the filesystem as it was.
func Open(cfg config.Config) (*Service, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	tree, err := workspace.Open(cfg.Root)
	if err != nil {
		return nil, err
	}
	issuer, err := auth.NewIssuer(cfg.TokenSecret, cfg.TokenTTL)
	if err != nil {
		return nil, err
	}
	var renderer *pdfgen.Runner
	if cfg.Renderer != "" {
		if renderer, err = pdfgen.New(cfg.Renderer); err != nil {
			return nil, err
		}
	}
	var deliveries *notify.Client
	if len(cfg.Partners) > 0 {
		guard, err := notify.NewGuard(cfg.Partners)
		if err != nil {
			return nil, err
		}
		if deliveries, err = notify.NewClient(guard, nil, cfg.MaxResponse, !cfg.DeliverInsecure); err != nil {
			return nil, err
		}
	}
	service := &Service{
		cfg:       cfg,
		workspace: tree,
		directory: identity.NewDirectory(),
		book:      pricing.NewBook(),
		store:     store.New(),
		trail:     journal.New(1),
		issuer:    issuer,
	}
	surface, err := api.New(api.Deps{
		Config:    cfg,
		Directory: service.directory,
		Book:      service.book,
		Store:     service.store,
		Journal:   service.trail,
		Workspace: tree,
		Renderer:  renderer,
		Notify:    deliveries,
		Issuer:    issuer,
	})
	if err != nil {
		return nil, err
	}
	service.api = surface
	return service, nil
}

// Handler returns the HTTP handler of the service.
func (s *Service) Handler(logger *log.Logger) http.Handler { return s.api.Handler(logger) }

// Routes lists what the service serves.
func (s *Service) Routes() []string { return s.api.Routes() }

// SetClock replaces the clock the responses are stamped and compared with.
func (s *Service) SetClock(now func() time.Time) { s.api.SetClock(now) }

// Root is the absolute root of the statement workspace.
func (s *Service) Root() string { return s.workspace.Root() }

// TrailLen reports how many audit entries the service carries.
func (s *Service) TrailLen() int { return s.trail.Len() }

// Issue signs a token for a subject. A deployment issues tokens from its identity provider; this
// method is how a test or a script mints one.
func (s *Service) Issue(subject, tenant string, scopes []string) string {
	return s.issuer.Issue(subject, tenant, scopes)
}

// Register adds one account to the directory. The account is validated by the directory, so a
// deployment cannot register an id that is not a statement-name segment.
func (s *Service) Register(account identity.Account) (identity.Account, error) {
	if !s.cfg.TenantAllowed(account.Tenant) {
		return identity.Account{}, identity.ErrInvalid
	}
	return s.directory.Add(account)
}

// AddRate registers one rate in the book.
func (s *Service) AddRate(tableID string, rate pricing.Rate) error {
	return s.book.Put(tableID, rate)
}
