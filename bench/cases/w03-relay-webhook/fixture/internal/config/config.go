// Package config loads and validates the configuration of a relay deployment.
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Errors the loader reports.
var (
	ErrInvalid = errors.New("config: invalid value")
	ErrMissing = errors.New("config: missing value")
)

// Config is the resolved configuration of one relay instance.
type Config struct {
	Addr        string
	Staging     string
	Allowlist   []string
	TokenSecret string
	MaxEntries  int
	MaxEntry    int64
	MaxTotal    int64
	Attempts    int
	Backoff     time.Duration
}

// Default is the configuration a bare deployment runs with.
func Default() Config {
	return Config{
		Addr:       ":8080",
		Staging:    "/var/lib/relay/staging",
		MaxEntries: 200,
		MaxEntry:   1 << 20,
		MaxTotal:   8 << 20,
		Attempts:   3,
		Backoff:    250 * time.Millisecond,
	}
}

// Load reads a configuration through a lookup function, the shape os.Getenv has.
func Load(lookup func(string) string) (Config, error) {
	cfg := Default()
	if v := strings.TrimSpace(lookup("RELAY_ADDR")); v != "" {
		cfg.Addr = v
	}
	if v := strings.TrimSpace(lookup("RELAY_STAGING")); v != "" {
		cfg.Staging = v
	}
	if v := strings.TrimSpace(lookup("RELAY_ALLOWLIST")); v != "" {
		for _, host := range strings.Split(v, ",") {
			if host = strings.TrimSpace(host); host != "" {
				cfg.Allowlist = append(cfg.Allowlist, host)
			}
		}
	}
	cfg.TokenSecret = strings.TrimSpace(lookup("RELAY_TOKEN_SECRET"))
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate refuses a configuration the service cannot run safely. A deployment with no allowlist
// would have no outbound policy at all, and a staging directory that is not absolute would resolve
// against whatever directory the process started in.
func (c Config) Validate() error {
	if strings.TrimSpace(c.Addr) == "" {
		return fmt.Errorf("%w: address", ErrMissing)
	}
	if !strings.HasPrefix(c.Staging, "/") {
		return fmt.Errorf("%w: staging %q is not absolute", ErrInvalid, c.Staging)
	}
	if c.Staging == "/" {
		return fmt.Errorf("%w: staging cannot be the filesystem root", ErrInvalid)
	}
	if len(c.Allowlist) == 0 {
		return fmt.Errorf("%w: outbound allowlist", ErrMissing)
	}
	for _, host := range c.Allowlist {
		if strings.ContainsAny(host, "/:@ ") {
			return fmt.Errorf("%w: allowlist entry %q is not a host", ErrInvalid, host)
		}
	}
	if len(c.TokenSecret) < 16 {
		return fmt.Errorf("%w: token secret shorter than 16 bytes", ErrInvalid)
	}
	if c.MaxEntries <= 0 || c.MaxEntry <= 0 || c.MaxTotal <= 0 {
		return fmt.Errorf("%w: stream limits must be positive", ErrInvalid)
	}
	if c.MaxEntry > c.MaxTotal {
		return fmt.Errorf("%w: an entry above the stream total", ErrInvalid)
	}
	if c.Attempts <= 0 || c.Attempts > 10 {
		return fmt.Errorf("%w: attempts %d", ErrInvalid, c.Attempts)
	}
	if c.Backoff <= 0 {
		return fmt.Errorf("%w: backoff %v", ErrInvalid, c.Backoff)
	}
	return nil
}
