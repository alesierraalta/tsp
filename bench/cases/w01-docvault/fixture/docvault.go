// Package docvault wires a document service from its options: configuration, the object tree, the
// token issuer and the HTTP surface. It is the package a deployment and a test both open the
// service through, so the wiring exists once.
package docvault

import (
	"log"
	"net/http"
	"time"

	"example.com/docvault/internal/api"
	"example.com/docvault/internal/auth"
	"example.com/docvault/internal/config"
	"example.com/docvault/internal/vault"
)

// Options are the values a deployment runs with. Zero values fall back to the defaults, and the
// combination is validated before anything is opened.
type Options struct {
	Root        string
	Addr        string
	Tenants     []string
	PageSize    int
	MaxUpload   int64
	TokenSecret string
	SessionTTL  time.Duration
}

// Service is one open document service.
type Service struct {
	cfg    config.Config
	vault  *vault.Vault
	issuer *auth.Issuer
	api    *api.API
}

// Open validates the options, opens the tree and wires the HTTP surface. Every failure leaves the
// filesystem untouched: the tree is opened read-only until the first write.
func Open(opts Options) (*Service, error) {
	cfg := config.Default()
	if opts.Root != "" {
		cfg.Root = opts.Root
	}
	if opts.Addr != "" {
		cfg.Addr = opts.Addr
	}
	if len(opts.Tenants) > 0 {
		cfg.Tenants = append([]string(nil), opts.Tenants...)
	}
	if opts.PageSize > 0 {
		cfg.PageSize = opts.PageSize
	}
	if opts.MaxUpload > 0 {
		cfg.MaxUpload = opts.MaxUpload
	}
	if opts.TokenSecret != "" {
		cfg.TokenSecret = opts.TokenSecret
	}
	if opts.SessionTTL > 0 {
		cfg.SessionTTL = opts.SessionTTL
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	tree, err := vault.Open(cfg.Root)
	if err != nil {
		return nil, err
	}
	issuer, err := auth.NewIssuer(cfg.TokenSecret)
	if err != nil {
		return nil, err
	}
	return &Service{cfg: cfg, vault: tree, issuer: issuer, api: api.New(cfg, tree, issuer)}, nil
}

// Handler returns the HTTP handler of the service.
func (s *Service) Handler(logger *log.Logger) http.Handler { return s.api.Handler(logger) }

// Routes lists what the service serves.
func (s *Service) Routes() []string { return s.api.Routes() }

// Addr is the address the deployment asked to listen on.
func (s *Service) Addr() string { return s.cfg.Addr }

// Root is the absolute root of the document tree.
func (s *Service) Root() string { return s.vault.Root() }

// SetClock replaces the clock the responses are stamped with.
func (s *Service) SetClock(now func() time.Time) { s.api.SetClock(now) }

// Issue signs a token for a subject in a tenant, with the scopes and lifetime given. A deployment
// issues tokens from its identity provider; this method is how a test or a script mints one.
func (s *Service) Issue(subject, tenant string, scopes []string, ttl time.Duration) string {
	return s.issuer.Issue(subject, tenant, scopes, ttl)
}

// TrailLen reports how many audit entries the service carries.
func (s *Service) TrailLen() int { return s.api.Trail().Len() }

// WriteObject stores an object the service serves, through the tree's containment rule. It is the
// call a deployment's pre-seed step uses to put the files the build produces in place.
func (s *Service) WriteObject(name string, data []byte) error { return s.vault.Write(name, data) }
