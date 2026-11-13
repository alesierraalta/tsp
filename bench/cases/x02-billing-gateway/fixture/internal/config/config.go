// Package config loads and validates what a deployment runs with: the listen address, the tenant
// list, the token secret, the currency, the cycle length, the request caps and the outbound
// endpoints of the gateway. A value is checked before anything is opened, and a refusal names the
// value, because an operator reading a start-up failure needs to know which line of the environment
// to change.
package config

import (
	"fmt"
	"strings"
	"time"
)

// Config is one validated deployment.
type Config struct {
	Addr           string
	Tenants        []string
	TokenSecret    string
	Currency       string
	CycleMonths    int
	SessionTTL     time.Duration
	RetentionDays  int
	GraceDays      int
	MaxPageSize    int
	MaxBodyBytes   int64
	MaxInvoiceRows int
	MaxEvents      int
	TaxRateBasis   int
	DefaultPlan    string
	AdminSubjects  []string
}

// Default is the configuration a deployment starts from, before the environment is overlaid.
func Default() Config {
	return Config{
		Addr:           ":8080",
		Currency:       "EUR",
		CycleMonths:    1,
		SessionTTL:     time.Hour,
		RetentionDays:  730,
		GraceDays:      7,
		MaxPageSize:    200,
		MaxBodyBytes:   1 << 20,
		MaxInvoiceRows: 500,
		MaxEvents:      100000,
		TaxRateBasis:   10000,
	}
}

// Overlay returns a copy of c with every field of over that is set. A zero field means "keep the
// default", so a caller can write only the values it cares about and never has to restate the rest.
func (c Config) Overlay(over Config) Config {
	if over.Addr != "" {
		c.Addr = over.Addr
	}
	if len(over.Tenants) > 0 {
		c.Tenants = append([]string(nil), over.Tenants...)
	}
	if over.TokenSecret != "" {
		c.TokenSecret = over.TokenSecret
	}
	if over.Currency != "" {
		c.Currency = over.Currency
	}
	if over.CycleMonths != 0 {
		c.CycleMonths = over.CycleMonths
	}
	if over.SessionTTL != 0 {
		c.SessionTTL = over.SessionTTL
	}
	if over.RetentionDays != 0 {
		c.RetentionDays = over.RetentionDays
	}
	if over.GraceDays != 0 {
		c.GraceDays = over.GraceDays
	}
	if over.MaxPageSize != 0 {
		c.MaxPageSize = over.MaxPageSize
	}
	if over.MaxBodyBytes != 0 {
		c.MaxBodyBytes = over.MaxBodyBytes
	}
	if over.MaxInvoiceRows != 0 {
		c.MaxInvoiceRows = over.MaxInvoiceRows
	}
	if over.MaxEvents != 0 {
		c.MaxEvents = over.MaxEvents
	}
	if over.TaxRateBasis != 0 {
		c.TaxRateBasis = over.TaxRateBasis
	}
	if over.DefaultPlan != "" {
		c.DefaultPlan = over.DefaultPlan
	}
	if len(over.AdminSubjects) > 0 {
		c.AdminSubjects = append([]string(nil), over.AdminSubjects...)
	}
	c.Currency = strings.ToUpper(c.Currency)
	c.Addr = strings.TrimSpace(c.Addr)
	return c
}

// Limits returns the request caps as their own value, so a handler carries the numbers it enforces
// instead of the whole configuration.
func (c Config) Limits() Limits {
	return Limits{
		PageSize:     c.MaxPageSize,
		BodyBytes:    c.MaxBodyBytes,
		InvoiceRows:  c.MaxInvoiceRows,
		EventCount:   c.MaxEvents,
		TaxRateBasis: c.TaxRateBasis,
	}
}

// RetentionGraceDays returns the number of days past an invoice's due date before a collection run
// moves it to past due.
func (c Config) RetentionGraceDays() int {
	if c.GraceDays < 0 {
		return 0
	}
	return c.GraceDays
}

// HasTenant reports whether id is one of the deployment's tenants. The comparison is byte for byte:
// two tenant ids that differ in any byte are two tenants.
func (c Config) HasTenant(id string) bool {
	for _, tenant := range c.Tenants {
		if tenant == id {
			return true
		}
	}
	return false
}

// IsAdmin reports whether a subject may call the operator routes.
func (c Config) IsAdmin(subject string) bool {
	for _, admin := range c.AdminSubjects {
		if admin == subject {
			return true
		}
	}
	return false
}

// Validate checks the whole configuration and returns the first problem it finds, naming the value.
func (c Config) Validate() error {
	if err := validateAddr(c.Addr); err != nil {
		return err
	}
	if err := validateTenants(c.Tenants); err != nil {
		return err
	}
	if err := validateSecret(c.TokenSecret); err != nil {
		return err
	}
	if err := validateCurrency(c.Currency); err != nil {
		return err
	}
	if c.CycleMonths < 1 || c.CycleMonths > 12 {
		return fmt.Errorf("config: BILLING_CYCLE_MONTHS is %d, want 1..12", c.CycleMonths)
	}
	if c.SessionTTL < time.Minute || c.SessionTTL > 30*24*time.Hour {
		return fmt.Errorf("config: BILLING_SESSION_TTL is %s, want 1m..720h", c.SessionTTL)
	}
	if c.RetentionDays < 30 {
		return fmt.Errorf("config: BILLING_RETENTION_DAYS is %d, want at least 30", c.RetentionDays)
	}
	if c.GraceDays < 0 || c.GraceDays > 90 {
		return fmt.Errorf("config: BILLING_GRACE_DAYS is %d, want 0..90", c.GraceDays)
	}
	if c.MaxPageSize < 1 || c.MaxPageSize > 1000 {
		return fmt.Errorf("config: BILLING_MAX_PAGE_SIZE is %d, want 1..1000", c.MaxPageSize)
	}
	if c.MaxBodyBytes < 1024 {
		return fmt.Errorf("config: BILLING_MAX_BODY is %d bytes, want at least 1024", c.MaxBodyBytes)
	}
	if c.MaxInvoiceRows < 1 {
		return fmt.Errorf("config: BILLING_MAX_INVOICE_ROWS is %d, want a positive count", c.MaxInvoiceRows)
	}
	if c.MaxEvents < 1 {
		return fmt.Errorf("config: BILLING_MAX_EVENTS is %d, want a positive count", c.MaxEvents)
	}
	if c.TaxRateBasis < 100 || c.TaxRateBasis > 100000 {
		return fmt.Errorf("config: BILLING_TAX_RATE_BASIS is %d, want 100..100000", c.TaxRateBasis)
	}
	return nil
}

// String renders the configuration for the console page, with the secret replaced by a marker: an
// operator reading the page must not be handed the key that signs every token.
func (c Config) String() string {
	return fmt.Sprintf("addr=%s tenants=%s currency=%s cycle=%dmo ttl=%s retention=%dd grace=%dd pagesize=%d body=%dB rows=%d events=%d secret=%s",
		c.Addr, strings.Join(c.Tenants, ","), c.Currency, c.CycleMonths, c.SessionTTL,
		c.RetentionDays, c.GraceDays, c.MaxPageSize, c.MaxBodyBytes, c.MaxInvoiceRows, c.MaxEvents,
		redact(c.TokenSecret))
}

// redact replaces a secret with a length marker. The length itself is not a secret worth hiding, and
// an operator diagnosing a truncated secret needs to see that it is short.
func redact(secret string) string {
	if secret == "" {
		return "(unset)"
	}
	return fmt.Sprintf("(%d bytes)", len(secret))
}
