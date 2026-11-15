// Package config loads and validates what an artifact server deployment runs with.
package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is one validated deployment.
type Config struct {
	Addr         string
	Root         string
	Projects     []string
	Allowlist    []string
	TokenSecret  string
	SessionTTL   time.Duration
	PageSize     int
	MaxBody      int64
	MaxArtifact  int64
	MaxArtifacts int
	Retention    int
	KeepLast     int
	Tools        []string
}

// Default is the configuration a deployment starts from.
func Default() Config {
	return Config{
		Addr:         ":8080",
		Root:         "",
		TokenSecret:  "",
		SessionTTL:   time.Hour,
		PageSize:     100,
		MaxBody:      16 << 20,
		MaxArtifact:  8 << 20,
		MaxArtifacts: 1000,
		Retention:    30,
		KeepLast:     5,
	}
}

// Overlay returns a copy of c with every field of over that is set.
func (c Config) Overlay(over Config) Config {
	if over.Addr != "" {
		c.Addr = over.Addr
	}
	if over.Root != "" {
		c.Root = over.Root
	}
	if len(over.Projects) > 0 {
		c.Projects = append([]string(nil), over.Projects...)
	}
	if len(over.Allowlist) > 0 {
		c.Allowlist = append([]string(nil), over.Allowlist...)
	}
	if over.TokenSecret != "" {
		c.TokenSecret = over.TokenSecret
	}
	if over.SessionTTL != 0 {
		c.SessionTTL = over.SessionTTL
	}
	if over.PageSize != 0 {
		c.PageSize = over.PageSize
	}
	if over.MaxBody != 0 {
		c.MaxBody = over.MaxBody
	}
	if over.MaxArtifact != 0 {
		c.MaxArtifact = over.MaxArtifact
	}
	if over.MaxArtifacts != 0 {
		c.MaxArtifacts = over.MaxArtifacts
	}
	if over.Retention != 0 {
		c.Retention = over.Retention
	}
	if over.KeepLast != 0 {
		c.KeepLast = over.KeepLast
	}
	if len(over.Tools) > 0 {
		c.Tools = append([]string(nil), over.Tools...)
	}
	return c
}

// HasProject reports whether a project is served by this deployment.
func (c Config) HasProject(project string) bool {
	for _, served := range c.Projects {
		if served == project {
			return true
		}
	}
	return false
}

// Validate checks the whole configuration.
func (c Config) Validate() error {
	if _, _, err := net.SplitHostPort(c.Addr); err != nil {
		return fmt.Errorf("config: ARTIFACTS_ADDR %q is not host:port", c.Addr)
	}
	if !strings.HasPrefix(c.Root, "/") || strings.TrimSpace(c.Root) == "/" {
		return fmt.Errorf("config: ARTIFACTS_ROOT %q must be an absolute directory that is not /", c.Root)
	}
	if len(c.Projects) == 0 {
		return fmt.Errorf("config: ARTIFACTS_PROJECTS is empty")
	}
	seen := make(map[string]bool, len(c.Projects))
	for _, project := range c.Projects {
		if !ValidName(project) {
			return fmt.Errorf("config: project %q is not a name", project)
		}
		if seen[project] {
			return fmt.Errorf("config: project %q is listed twice", project)
		}
		seen[project] = true
	}
	for _, host := range c.Allowlist {
		if strings.ContainsAny(host, "/:@ ") || host == "" {
			return fmt.Errorf("config: ARTIFACTS_ALLOWLIST carries %q, which is not a host", host)
		}
	}
	if len(c.TokenSecret) < 16 {
		return fmt.Errorf("config: ARTIFACTS_TOKEN_SECRET is %d bytes, want at least 16", len(c.TokenSecret))
	}
	if c.SessionTTL < time.Minute || c.SessionTTL > 30*24*time.Hour {
		return fmt.Errorf("config: ARTIFACTS_SESSION_TTL is %s", c.SessionTTL)
	}
	if c.PageSize < 1 || c.PageSize > 1000 {
		return fmt.Errorf("config: ARTIFACTS_PAGE_SIZE is %d", c.PageSize)
	}
	if c.MaxBody < 1024 || c.MaxBody > 1<<30 {
		return fmt.Errorf("config: ARTIFACTS_MAX_BODY is %d bytes", c.MaxBody)
	}
	if c.MaxArtifact < 1 || c.MaxArtifact > c.MaxBody {
		return fmt.Errorf("config: ARTIFACTS_MAX_ARTIFACT is %d bytes", c.MaxArtifact)
	}
	if c.MaxArtifacts < 1 {
		return fmt.Errorf("config: ARTIFACTS_MAX_ARTIFACTS is %d", c.MaxArtifacts)
	}
	if c.Retention < 1 || c.Retention > 3650 {
		return fmt.Errorf("config: ARTIFACTS_RETENTION_DAYS is %d", c.Retention)
	}
	if c.KeepLast < 1 || c.KeepLast > c.MaxArtifacts {
		return fmt.Errorf("config: ARTIFACTS_KEEP_LAST is %d", c.KeepLast)
	}
	return nil
}

// ValidName reports whether a project or a tool is a lower-case name.
func ValidName(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	if c := value[0]; c < 'a' || c > 'z' {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// String renders the configuration for the console, with the secret replaced by its length.
func (c Config) String() string {
	return fmt.Sprintf("addr=%s root=%s projects=%s allowlist=%s ttl=%s pagesize=%d body=%dB artifact=%dB artifacts=%d retention=%dd keep=%d secret=(%d bytes)",
		c.Addr, c.Root, strings.Join(c.Projects, ","), strings.Join(c.Allowlist, ","),
		c.SessionTTL, c.PageSize, c.MaxBody, c.MaxArtifact, c.MaxArtifacts, c.Retention, c.KeepLast,
		len(c.TokenSecret))
}

// Env is the environment a deployment reads.
type Env func(key string) string

// OS is the process environment.
func OS() Env { return os.Getenv }

// FromMap returns an Env over a map.
func FromMap(values map[string]string) Env {
	return func(key string) string { return values[key] }
}

// Load overlays the environment onto the defaults and validates the result.
func Load(get Env) (Config, error) {
	cfg := Default()
	cfg.Addr = first(get("ARTIFACTS_ADDR"), cfg.Addr)
	cfg.Root = first(get("ARTIFACTS_ROOT"), cfg.Root)
	cfg.TokenSecret = first(get("ARTIFACTS_TOKEN_SECRET"), cfg.TokenSecret)
	if value := get("ARTIFACTS_PROJECTS"); value != "" {
		cfg.Projects = split(value)
	}
	if value := get("ARTIFACTS_ALLOWLIST"); value != "" {
		cfg.Allowlist = split(value)
	}
	if value := get("ARTIFACTS_TOOLS"); value != "" {
		cfg.Tools = split(value)
	}
	var err error
	if cfg.SessionTTL, err = duration(get, "ARTIFACTS_SESSION_TTL", cfg.SessionTTL); err != nil {
		return Config{}, err
	}
	if cfg.PageSize, err = number(get, "ARTIFACTS_PAGE_SIZE", cfg.PageSize); err != nil {
		return Config{}, err
	}
	if cfg.MaxArtifacts, err = number(get, "ARTIFACTS_MAX_ARTIFACTS", cfg.MaxArtifacts); err != nil {
		return Config{}, err
	}
	if cfg.Retention, err = number(get, "ARTIFACTS_RETENTION_DAYS", cfg.Retention); err != nil {
		return Config{}, err
	}
	if cfg.KeepLast, err = number(get, "ARTIFACTS_KEEP_LAST", cfg.KeepLast); err != nil {
		return Config{}, err
	}
	if cfg.MaxBody, err = bytes(get, "ARTIFACTS_MAX_BODY", cfg.MaxBody); err != nil {
		return Config{}, err
	}
	if cfg.MaxArtifact, err = bytes(get, "ARTIFACTS_MAX_ARTIFACT", cfg.MaxArtifact); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// first returns value when it is set, fallback otherwise.
func first(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

// split splits a comma-separated list and drops empty entries.
func split(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// number reads an integer variable.
func number(get Env, key string, fallback int) (int, error) {
	value := strings.TrimSpace(get(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("config: %s=%q is not a number", key, value)
	}
	return parsed, nil
}

// bytes reads a byte size with an optional k, m or g suffix.
func bytes(get Env, key string, fallback int64) (int64, error) {
	value := strings.TrimSpace(get(key))
	if value == "" {
		return fallback, nil
	}
	multiplier := int64(1)
	switch {
	case strings.HasSuffix(value, "k"), strings.HasSuffix(value, "K"):
		multiplier, value = 1<<10, value[:len(value)-1]
	case strings.HasSuffix(value, "m"), strings.HasSuffix(value, "M"):
		multiplier, value = 1<<20, value[:len(value)-1]
	case strings.HasSuffix(value, "g"), strings.HasSuffix(value, "G"):
		multiplier, value = 1<<30, value[:len(value)-1]
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("config: %s=%q is not a byte size", key, get(key))
	}
	return parsed * multiplier, nil
}

// duration reads a duration variable.
func duration(get Env, key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(get(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("config: %s=%q is not a positive duration", key, value)
	}
	return parsed, nil
}
