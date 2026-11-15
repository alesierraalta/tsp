// Package config loads and validates the docvault service configuration.
package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Errors the loader reports. Callers compare with errors.Is.
var (
	ErrInvalid = errors.New("config: invalid value")
	ErrMissing = errors.New("config: missing value")
)

// Limits are the request caps every handler applies.
type Limits struct {
	// PageSize is the largest page a list endpoint returns.
	PageSize int
	// MaxUpload is the largest object the upload endpoint accepts, in bytes.
	MaxUpload int64
	// MaxBody is the largest request body the API decodes, in bytes.
	MaxBody int64
}

// Config is the resolved configuration of one docvault instance.
type Config struct {
	Root        string
	Addr        string
	Tenants     []string
	PageSize    int
	MaxUpload   int64
	TokenSecret string
	SessionTTL  time.Duration
}

// Default is the configuration a bare service runs with.
func Default() Config {
	return Config{
		Root:       "/srv/docvault",
		Addr:       ":8080",
		PageSize:   50,
		MaxUpload:  8 << 20,
		SessionTTL: 12 * time.Hour,
	}
}

// Load reads a configuration through a lookup function, the shape os.Getenv has, so a test can
// drive the loader without touching the process environment.
func Load(lookup func(string) string) (Config, error) {
	cfg := Default()
	if v := strings.TrimSpace(lookup("DOCVAULT_ROOT")); v != "" {
		cfg.Root = v
	}
	if v := strings.TrimSpace(lookup("DOCVAULT_ADDR")); v != "" {
		cfg.Addr = v
	}
	if v := strings.TrimSpace(lookup("DOCVAULT_TENANTS")); v != "" {
		for _, t := range strings.Split(v, ",") {
			if t = strings.TrimSpace(t); t != "" {
				cfg.Tenants = append(cfg.Tenants, t)
			}
		}
	}
	if v := strings.TrimSpace(lookup("DOCVAULT_PAGE_SIZE")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("%w: DOCVAULT_PAGE_SIZE=%q", ErrInvalid, v)
		}
		cfg.PageSize = n
	}
	if v := strings.TrimSpace(lookup("DOCVAULT_MAX_UPLOAD")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("%w: DOCVAULT_MAX_UPLOAD=%q", ErrInvalid, v)
		}
		cfg.MaxUpload = n
	}
	if v := strings.TrimSpace(lookup("DOCVAULT_SESSION_TTL")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("%w: DOCVAULT_SESSION_TTL=%q", ErrInvalid, v)
		}
		cfg.SessionTTL = d
	}
	cfg.TokenSecret = strings.TrimSpace(lookup("DOCVAULT_TOKEN_SECRET"))
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate refuses the configurations the service cannot run safely: a service without a secret
// would accept tokens it never signed, and a service without tenants has no access boundary.
func (c Config) Validate() error {
	if strings.TrimSpace(c.Root) == "" {
		return fmt.Errorf("%w: root", ErrMissing)
	}
	if !strings.HasPrefix(c.Root, "/") {
		return fmt.Errorf("%w: root %q is not absolute", ErrInvalid, c.Root)
	}
	if c.Root == "/" {
		return fmt.Errorf("%w: root of the tree cannot be the filesystem root", ErrInvalid)
	}
	if len(c.TokenSecret) < 16 {
		return fmt.Errorf("%w: token secret shorter than 16 bytes", ErrInvalid)
	}
	if c.PageSize <= 0 {
		return fmt.Errorf("%w: page size %d", ErrInvalid, c.PageSize)
	}
	if c.PageSize > HardPageSize {
		return fmt.Errorf("%w: page size %d above %d", ErrInvalid, c.PageSize, HardPageSize)
	}
	if c.MaxUpload <= 0 {
		return fmt.Errorf("%w: max upload %d", ErrInvalid, c.MaxUpload)
	}
	if c.MaxUpload > HardMaxUpload {
		return fmt.Errorf("%w: max upload %d above %d", ErrInvalid, c.MaxUpload, HardMaxUpload)
	}
	if c.SessionTTL <= 0 {
		return fmt.Errorf("%w: session ttl %v", ErrInvalid, c.SessionTTL)
	}
	if len(c.Tenants) == 0 {
		return fmt.Errorf("%w: at least one tenant", ErrMissing)
	}
	for _, t := range c.Tenants {
		if strings.TrimSpace(t) == "" || strings.ToLower(t) != t {
			return fmt.Errorf("%w: tenant %q is not a lowercase id", ErrInvalid, t)
		}
	}
	return nil
}

// Limits reports the request caps as a value the handlers carry.
func (c Config) Limits() Limits {
	return Limits{PageSize: c.PageSize, MaxUpload: c.MaxUpload, MaxBody: c.MaxUpload + 4096}
}

// TenantAllowed reports whether the configuration knows a tenant.
func (c Config) TenantAllowed(tenant string) bool {
	for _, t := range c.Tenants {
		if t == tenant {
			return true
		}
	}
	return false
}
