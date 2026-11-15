package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// StagingPath is the directory the batch staging trees live under. It is derived from the root so a
// deployment cannot point it somewhere the containment rule does not cover.
func (c Config) StagingPath() string {
	if c.Root == "" {
		return ""
	}
	return filepath.Join(c.Root, ".staging")
}

// StorePath is the directory the committed objects live under.
func (c Config) StorePath() string {
	if c.Root == "" {
		return ""
	}
	return filepath.Join(c.Root, "objects")
}

// Limits is the caps a request is held to, derived from the configuration so the request path and
// the manifest parser read the same numbers.
func (c Config) Limits() Limits {
	return Limits{
		MaxUpload:     c.MaxUpload,
		MaxEntries:    c.MaxEntries,
		MaxPages:      c.MaxPages,
		MaxExportRows: c.MaxExportRows,
		MaxBodyBytes:  c.MaxUpload + (1 << 20),
		MaxNameBytes:  128,
	}
}

// AddrHost is the host part of the listen address, for the startup log line.
func (c Config) AddrHost() string {
	host, _, ok := strings.Cut(c.Addr, ":")
	if !ok {
		return c.Addr
	}
	return host
}

// TenantList renders the tenants the way the startup log and the admin page print them.
func (c Config) TenantList() string { return strings.Join(c.Tenants, ",") }

// Tenant reports whether id is one of the configured tenants. The comparison is exact: two ids that
// differ in any byte are two tenants, and a caller cannot normalize its way into another one.
func (c Config) Tenant(id string) bool {
	for _, t := range c.Tenants {
		if t == id {
			return true
		}
	}
	return false
}

// String renders the configuration for a log line, without the token secret.
func (c Config) String() string {
	return fmt.Sprintf("root=%s addr=%s tenants=%s page=%d upload=%d retention=%dd ttl=%s",
		c.Root, c.Addr, c.TenantList(), c.PageSize, c.MaxUpload, c.RetentionDays, c.SessionTTL)
}
