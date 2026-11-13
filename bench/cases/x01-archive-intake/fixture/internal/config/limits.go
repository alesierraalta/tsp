package config

import (
	"fmt"
	"strconv"
	"strings"
)

// Limits are the caps every request and every manifest is held to. They are numbers a deployment
// sets, and they are read in exactly one place each so a handler cannot grow its own.
type Limits struct {
	// MaxUpload is the largest object in bytes.
	MaxUpload int64
	// MaxEntries is the largest manifest entry count.
	MaxEntries int
	// MaxPages is the largest page count one entry may declare.
	MaxPages int
	// MaxExportRows is the largest number of rows one CSV export writes.
	MaxExportRows int
	// MaxBodyBytes is the largest request body the decoder will read.
	MaxBodyBytes int64
	// MaxNameBytes is the largest name the store accepts.
	MaxNameBytes int
}

// DefaultLimits is the cap set a zero Limits falls back to. It is the same as Default, so a caller
// that skipped the configuration still runs with the shipped caps rather than with none.
func DefaultLimits() Limits { return Default().Limits() }

// Or returns l, or the defaults when l carries no caps at all. A partially filled Limits is used as
// given: a deployment that raised one cap meant to.
func (l Limits) Or() Limits {
	if l == (Limits{}) {
		return DefaultLimits()
	}
	return l
}

// Check refuses a cap set that cannot bound anything. The check runs at startup, so a request never
// has to guess what a zero cap means.
func (l Limits) Check() error {
	var problems []string
	if l.MaxUpload <= 0 {
		problems = append(problems, "upload cap must be positive")
	}
	if l.MaxEntries <= 0 {
		problems = append(problems, "entry cap must be positive")
	}
	if l.MaxPages <= 0 {
		problems = append(problems, "page cap must be positive")
	}
	if l.MaxExportRows <= 0 {
		problems = append(problems, "export row cap must be positive")
	}
	if l.MaxBodyBytes < l.MaxUpload {
		problems = append(problems, "body cap is smaller than the upload cap")
	}
	if l.MaxNameBytes <= 0 {
		problems = append(problems, "name cap must be positive")
	}
	if len(problems) > 0 {
		return fmt.Errorf("config: limits: %s", strings.Join(problems, "; "))
	}
	return nil
}

// ParseBytes reads a size with an optional unit suffix: 32M, 4Mi, 512k, 1024. The suffix is
// case-insensitive and "i" means 1024, so 4Mi and 4M both mean four mebibytes here.
func ParseBytes(s string) (int64, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return 0, fmt.Errorf("config: empty size")
	}
	mult := int64(1)
	lower := strings.ToLower(trimmed)
	for _, suffix := range []struct {
		mark string
		mult int64
	}{
		{"gi", 1 << 30}, {"g", 1 << 30},
		{"mi", 1 << 20}, {"m", 1 << 20},
		{"ki", 1 << 10}, {"k", 1 << 10},
	} {
		if strings.HasSuffix(lower, suffix.mark) {
			mult = suffix.mult
			trimmed = strings.TrimSpace(trimmed[:len(trimmed)-len(suffix.mark)])
			break
		}
	}
	n, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("config: size %q: %w", s, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("config: size %q is negative", s)
	}
	if n > (1<<62)/mult {
		return 0, fmt.Errorf("config: size %q overflows", s)
	}
	return n * mult, nil
}

// ParseCount reads a positive count: a page size, an entry cap, a retention window in days.
func ParseCount(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("config: count %q: %w", s, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("config: count %q must be positive", s)
	}
	return n, nil
}

// ParseTenants reads a comma-separated tenant list, refusing duplicates and empty entries. The ids
// are kept exactly as written, because the comparison the service makes is exact.
func ParseTenants(s string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		id := strings.TrimSpace(part)
		if id == "" {
			return nil, fmt.Errorf("config: empty tenant id in %q", s)
		}
		if seen[id] {
			return nil, fmt.Errorf("config: tenant %q repeated", id)
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("config: no tenants")
	}
	return out, nil
}
