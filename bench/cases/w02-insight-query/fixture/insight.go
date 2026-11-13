// Package insight wires the analytics service: the store, the account directory, the report
// registry and the HTTP surface.
package insight

import (
	"net/http"
	"time"

	"example.com/insight/internal/access"
	"example.com/insight/internal/httpapi"
	"example.com/insight/internal/reports"
	"example.com/insight/internal/store"
)

// Options are the values a deployment runs with.
type Options struct {
	TokenSecret string
	// Seed loads the sample account, event and quota rows a fresh instance serves.
	Seed bool
}

// Service is one open analytics service.
type Service struct {
	store     *store.Store
	directory *access.Directory
	registry  *reports.Registry
	issuer    *access.Issuer
	api       *httpapi.API
}

// Open builds a service from its options. The directory is built from the store's account rows, so
// an account exists in one place and the authorization path reads it from there.
func Open(opts Options) (*Service, error) {
	st := store.Open()
	if opts.Seed {
		if err := st.Seed(); err != nil {
			return nil, err
		}
	}
	dir := access.NewDirectory()
	accounts, err := st.Accounts()
	if err != nil {
		return nil, err
	}
	for _, a := range accounts {
		if err := dir.Add(access.Account{Subject: a.Subject, Tenant: a.Tenant, Display: a.Display, Role: a.Role}); err != nil {
			return nil, err
		}
	}
	issuer, err := access.NewIssuer(opts.TokenSecret)
	if err != nil {
		return nil, err
	}
	registry := reports.NewRegistry()
	return &Service{
		store:     st,
		directory: dir,
		registry:  registry,
		issuer:    issuer,
		api:       httpapi.New(st, registry, dir, issuer),
	}, nil
}

// Handler returns the HTTP handler of the service.
func (s *Service) Handler() http.Handler { return s.api.Handler() }

// Routes lists what the service serves.
func (s *Service) Routes() []string { return s.api.Routes() }

// SetClock replaces the clock the responses are stamped with.
func (s *Service) SetClock(now func() time.Time) { s.api.SetClock(now) }

// Issue signs a token for a subject in a tenant, with the scopes and lifetime given.
func (s *Service) Issue(subject, tenant string, scopes []string, ttl time.Duration) string {
	return s.issuer.Issue(subject, tenant, scopes, ttl)
}

// ReportNames lists the reports the service serves.
func (s *Service) ReportNames() []string { return s.registry.Names() }

// ExportCSV renders the events of a tenant as the export endpoint does.
func (s *Service) ExportCSV(tenant, status string) (string, error) {
	return reports.ExportCSV(s.store.DB(), tenant, status)
}
