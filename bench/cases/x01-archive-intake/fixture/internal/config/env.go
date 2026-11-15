package config

import (
	"fmt"
	"strconv"
	"strings"
)

// Load reads a configuration from an environment slice ("KEY=value" pairs, as os.Environ returns
// them). Unknown keys are ignored: the environment of a process carries more than this service.
func Load(environ []string) (Config, error) {
	cfg := Default()
	get := func(key string) (string, bool) {
		prefix := key + "="
		for _, kv := range environ {
			if strings.HasPrefix(kv, prefix) {
				return kv[len(prefix):], true
			}
		}
		return "", false
	}
	set := func(key string, target *string) {
		if v, ok := get(key); ok {
			*target = strings.TrimSpace(v)
		}
	}
	if v, ok := get("ARCHIVE_ROOT"); ok {
		cfg.Root = strings.TrimSpace(v)
	}
	set("ARCHIVE_ADDR", &cfg.Addr)
	set("ARCHIVE_TOKEN_SECRET", &cfg.TokenSecret)
	set("ARCHIVE_ZONE", &cfg.Zone)
	if v, ok := get("ARCHIVE_TENANTS"); ok {
		tenants, err := ParseTenants(v)
		if err != nil {
			return Config{}, err
		}
		cfg.Tenants = tenants
	}
	if v, ok := get("ARCHIVE_PAGE_SIZE"); ok {
		n, err := ParseCount(v)
		if err != nil {
			return Config{}, err
		}
		cfg.PageSize = n
	}
	if v, ok := get("ARCHIVE_MAX_UPLOAD"); ok {
		n, err := ParseBytes(v)
		if err != nil {
			return Config{}, err
		}
		cfg.MaxUpload = n
	}
	if v, ok := get("ARCHIVE_MAX_ENTRIES"); ok {
		n, err := ParseCount(v)
		if err != nil {
			return Config{}, err
		}
		cfg.MaxEntries = n
	}
	if v, ok := get("ARCHIVE_MAX_PAGES"); ok {
		n, err := ParseCount(v)
		if err != nil {
			return Config{}, err
		}
		cfg.MaxPages = n
	}
	if v, ok := get("ARCHIVE_MAX_EXPORT_ROWS"); ok {
		n, err := ParseCount(v)
		if err != nil {
			return Config{}, err
		}
		cfg.MaxExportRows = n
	}
	if v, ok := get("ARCHIVE_SESSION_TTL"); ok {
		d, err := ParseTTL(v)
		if err != nil {
			return Config{}, err
		}
		cfg.SessionTTL = d
	}
	if v, ok := get("ARCHIVE_RETENTION_DAYS"); ok {
		n, err := ParseCount(v)
		if err != nil {
			return Config{}, err
		}
		cfg.RetentionDays = n
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Overlay applies the non-zero values of over onto a copy of c. It is what the root package uses for
// its Options, so a caller sets only what it cares about and the defaults hold the rest.
func (c Config) Overlay(over Config) Config {
	out := c.Clone()
	if over.Root != "" {
		out.Root = over.Root
	}
	if over.Addr != "" {
		out.Addr = over.Addr
	}
	if len(over.Tenants) > 0 {
		out.Tenants = append([]string(nil), over.Tenants...)
	}
	if over.PageSize > 0 {
		out.PageSize = over.PageSize
	}
	if over.MaxUpload > 0 {
		out.MaxUpload = over.MaxUpload
	}
	if over.MaxEntries > 0 {
		out.MaxEntries = over.MaxEntries
	}
	if over.MaxPages > 0 {
		out.MaxPages = over.MaxPages
	}
	if over.MaxExportRows > 0 {
		out.MaxExportRows = over.MaxExportRows
	}
	if over.TokenSecret != "" {
		out.TokenSecret = over.TokenSecret
	}
	if over.SessionTTL > 0 {
		out.SessionTTL = over.SessionTTL
	}
	if over.RetentionDays > 0 {
		out.RetentionDays = over.RetentionDays
	}
	if over.Zone != "" {
		out.Zone = over.Zone
	}
	return out
}

// Env renders the configuration as the environment it came from, for the operator page's "running
// with" block. The token secret is never rendered.
func (c Config) Env() []string {
	out := []string{
		"ARCHIVE_ROOT=" + c.Root,
		"ARCHIVE_ADDR=" + c.Addr,
		"ARCHIVE_TENANTS=" + c.TenantList(),
		"ARCHIVE_PAGE_SIZE=" + strconv.Itoa(c.PageSize),
		"ARCHIVE_MAX_UPLOAD=" + strconv.FormatInt(c.MaxUpload, 10),
		"ARCHIVE_MAX_ENTRIES=" + strconv.Itoa(c.MaxEntries),
		"ARCHIVE_MAX_PAGES=" + strconv.Itoa(c.MaxPages),
		"ARCHIVE_SESSION_TTL=" + c.SessionTTL.String(),
		"ARCHIVE_RETENTION_DAYS=" + strconv.Itoa(c.RetentionDays),
		"ARCHIVE_ZONE=" + c.Zone,
	}
	return out
}

// RequireEnv reads a variable that has no default and refuses an empty value, naming the key.
func RequireEnv(environ []string, key string) (string, error) {
	prefix := key + "="
	for _, kv := range environ {
		if strings.HasPrefix(kv, prefix) {
			value := strings.TrimSpace(kv[len(prefix):])
			if value == "" {
				return "", fmt.Errorf("config: %s is set but empty", key)
			}
			return value, nil
		}
	}
	return "", fmt.Errorf("config: %s is not set", key)
}
