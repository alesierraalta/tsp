package config

import (
	"time"
)

// Package config loads and validates the values one deployment runs with. Loading happens before
// anything is opened, so a bad value fails the process instead of a request.

// Config is the whole configuration of one archive deployment. The zero value is not usable; use
// Default and overlay what the environment supplies.
type Config struct {
	// Root is the absolute directory the archive owns. Every path the service touches resolves
	// inside it, and the staging tree lives in a directory of its own below it.
	Root string
	// Addr is the listen address of the HTTP surface.
	Addr string
	// Tenants are the tenant ids the service accepts, lowercase, byte for byte.
	Tenants []string
	// PageSize is the default number of rows a list response carries.
	PageSize int
	// MaxUpload is the largest object, in bytes, that an upload or a scan may deliver.
	MaxUpload int64
	// MaxEntries is the largest number of manifest entries one batch may carry.
	MaxEntries int
	// MaxPages is the largest page count one entry may declare.
	MaxPages int
	// MaxExportRows is the largest number of rows the CSV export will write.
	MaxExportRows int
	// TokenSecret signs issued tokens. It is at least sixteen bytes.
	TokenSecret string
	// SessionTTL is how long an issued token stays valid.
	SessionTTL time.Duration
	// RetentionDays is how many whole UTC days a closed batch is kept before a sweep may remove it.
	RetentionDays int
	// Zone is the reporting zone of the operator pages. Stored instants stay UTC.
	Zone string
}

// Default is the configuration a deployment with no environment gets. The root is empty on purpose:
// a deployment that does not say where the archive lives must not start.
func Default() Config {
	return Config{
		Root:          "",
		Addr:          "127.0.0.1:8080",
		Tenants:       []string{"acme"},
		PageSize:      25,
		MaxUpload:     32 << 20,
		MaxEntries:    500,
		MaxPages:      2000,
		MaxExportRows: 10000,
		TokenSecret:   "",
		SessionTTL:    time.Hour,
		RetentionDays: 30,
		Zone:          "UTC",
	}
}

// Clone returns a copy a caller may modify without touching the original's slice fields.
func (c Config) Clone() Config {
	c.Tenants = append([]string(nil), c.Tenants...)
	return c
}
