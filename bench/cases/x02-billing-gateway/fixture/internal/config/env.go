package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Env is the environment this deployment reads its values from, as a function, so a test hands over
// a map and a process reads its own environment through the same loader.
type Env func(key string) string

// OS is the process environment.
func OS() Env { return os.Getenv }

// FromMap returns an Env that reads a map, for tests and for a second configuration.
func FromMap(values map[string]string) Env {
	return func(key string) string { return values[key] }
}

// Load overlays the environment onto base and validates the result. Every error names the variable
// the operator has to change, not the field the parser wanted.
func Load(get Env) (Config, error) {
	cfg := Default()
	if err := readInto(&cfg, get); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// readInto reads each variable the deployment knows and writes it into cfg.
func readInto(cfg *Config, get Env) error {
	var err error
	if value := get("BILLING_ADDR"); value != "" {
		cfg.Addr = value
	}
	if value := get("BILLING_TENANTS"); value != "" {
		cfg.Tenants = splitList(value)
	}
	if value := get("BILLING_TOKEN_SECRET"); value != "" {
		cfg.TokenSecret = value
	}
	if value := get("BILLING_CURRENCY"); value != "" {
		cfg.Currency = strings.ToUpper(value)
	}
	if value := get("BILLING_ADMIN_SUBJECTS"); value != "" {
		cfg.AdminSubjects = splitList(value)
	}
	if value := get("BILLING_DEFAULT_PLAN"); value != "" {
		cfg.DefaultPlan = value
	}
	if cfg.CycleMonths, err = readInt(get, "BILLING_CYCLE_MONTHS", cfg.CycleMonths); err != nil {
		return err
	}
	if cfg.RetentionDays, err = readInt(get, "BILLING_RETENTION_DAYS", cfg.RetentionDays); err != nil {
		return err
	}
	if cfg.GraceDays, err = readInt(get, "BILLING_GRACE_DAYS", cfg.GraceDays); err != nil {
		return err
	}
	if cfg.MaxPageSize, err = readInt(get, "BILLING_MAX_PAGE_SIZE", cfg.MaxPageSize); err != nil {
		return err
	}
	if cfg.MaxInvoiceRows, err = readInt(get, "BILLING_MAX_INVOICE_ROWS", cfg.MaxInvoiceRows); err != nil {
		return err
	}
	if cfg.MaxEvents, err = readInt(get, "BILLING_MAX_EVENTS", cfg.MaxEvents); err != nil {
		return err
	}
	if cfg.TaxRateBasis, err = readInt(get, "BILLING_TAX_RATE_BASIS", cfg.TaxRateBasis); err != nil {
		return err
	}
	if cfg.MaxBodyBytes, err = readBytes(get, "BILLING_MAX_BODY", cfg.MaxBodyBytes); err != nil {
		return err
	}
	if cfg.SessionTTL, err = readDuration(get, "BILLING_SESSION_TTL", cfg.SessionTTL); err != nil {
		return err
	}
	return nil
}

// splitList splits a comma-separated environment value, dropping empty entries so a trailing comma
// is not a tenant.
func splitList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// readInt reads an integer variable, leaving fallback in place when the variable is unset.
func readInt(get Env, key string, fallback int) (int, error) {
	value := get(key)
	if value == "" {
		return fallback, nil
	}
	number, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("config: %s=%q is not a number", key, value)
	}
	return number, nil
}

// readBytes reads a byte size. A bare number is bytes; a suffix of k, m or g multiplies by 1024.
func readBytes(get Env, key string, fallback int64) (int64, error) {
	value := strings.TrimSpace(get(key))
	if value == "" {
		return fallback, nil
	}
	multiplier := int64(1)
	switch {
	case strings.HasSuffix(value, "k"), strings.HasSuffix(value, "K"):
		multiplier = 1 << 10
		value = value[:len(value)-1]
	case strings.HasSuffix(value, "m"), strings.HasSuffix(value, "M"):
		multiplier = 1 << 20
		value = value[:len(value)-1]
	case strings.HasSuffix(value, "g"), strings.HasSuffix(value, "G"):
		multiplier = 1 << 30
		value = value[:len(value)-1]
	}
	number, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || number < 0 {
		return 0, fmt.Errorf("config: %s=%q is not a byte size", key, get(key))
	}
	if number > (1<<62)/multiplier {
		return 0, fmt.Errorf("config: %s=%q is too large", key, get(key))
	}
	return number * multiplier, nil
}

// readDuration reads a duration variable in Go's duration syntax.
func readDuration(get Env, key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(get(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("config: %s=%q is not a duration", key, value)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("config: %s=%q is not a positive duration", key, value)
	}
	return parsed, nil
}
