// Package config loads and validates the configuration of one ledgerclose service. Every value is
// read once, at startup, and a configuration the service cannot run safely is refused before a
// listener opens: a service without a token secret would accept tokens it never signed, and a close
// window of zero would let an operator re-bill a month that is still open.
package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"example.com/ledgerclose/internal/calendar"
	"example.com/ledgerclose/internal/money"
	"example.com/ledgerclose/internal/usage"
)

// Errors the loader reports. Callers compare with errors.Is.
var (
	ErrInvalid = errors.New("config: invalid value")
	ErrMissing = errors.New("config: missing value")
)

// Config is the resolved configuration of one ledgerclose instance.
type Config struct {
	Root             string
	Addr             string
	Tenants          []string
	PageSize         int
	MaxBatch         int
	MaxBody          int64
	MaxEventQuantity int64
	MaxUnitAmount    money.Amount
	TokenSecret      string
	TokenTTL         time.Duration
	CloseWindow      int
	Renderer         string
	Partners         []string
	DeliverInsecure  bool
	MaxResponse      int64
	TaxBasisPoints   int64
	MaxLines         int
}

// Default is the configuration a bare service runs with. It carries no tenants and no secret, which
// Validate refuses: a deployment has to say who it bills and what signed its tokens.
func Default() Config {
	return Config{
		Root: "/srv/ledgerclose", Addr: ":8080", PageSize: 50, MaxBatch: 500, MaxBody: 1 << 20,
		MaxEventQuantity: 1_000_000, MaxUnitAmount: 1_000_000_00, TokenTTL: 12 * time.Hour,
		CloseWindow: 3, MaxResponse: 64 << 10, MaxLines: 5_000,
	}
}

// Load reads a configuration through a lookup function, the shape os.LookupEnv has, so a test can
// drive the loader without touching the process environment.
func Load(lookup func(string) string) (Config, error) {
	cfg := Default()
	if v := strings.TrimSpace(lookup("LEDGERCLOSE_ROOT")); v != "" {
		cfg.Root = v
	}
	if v := strings.TrimSpace(lookup("LEDGERCLOSE_ADDR")); v != "" {
		cfg.Addr = v
	}
	if v := strings.TrimSpace(lookup("LEDGERCLOSE_TENANTS")); v != "" {
		cfg.Tenants = split(v)
	}
	if v := strings.TrimSpace(lookup("LEDGERCLOSE_PARTNERS")); v != "" {
		cfg.Partners = split(v)
	}
	for name, into := range map[string]*int{
		"LEDGERCLOSE_PAGE_SIZE": &cfg.PageSize, "LEDGERCLOSE_MAX_BATCH": &cfg.MaxBatch,
		"LEDGERCLOSE_CLOSE_WINDOW": &cfg.CloseWindow, "LEDGERCLOSE_MAX_LINES": &cfg.MaxLines,
	} {
		if v := strings.TrimSpace(lookup(name)); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				return Config{}, fmt.Errorf("%w: %s=%q", ErrInvalid, name, v)
			}
			*into = n
		}
	}
	for name, into := range map[string]*int64{
		"LEDGERCLOSE_MAX_BODY": &cfg.MaxBody, "LEDGERCLOSE_MAX_EVENT_QUANTITY": &cfg.MaxEventQuantity,
		"LEDGERCLOSE_MAX_RESPONSE": &cfg.MaxResponse, "LEDGERCLOSE_TAX_BASIS_POINTS": &cfg.TaxBasisPoints,
	} {
		if v := strings.TrimSpace(lookup(name)); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return Config{}, fmt.Errorf("%w: %s=%q", ErrInvalid, name, v)
			}
			*into = n
		}
	}
	if v := strings.TrimSpace(lookup("LEDGERCLOSE_MAX_UNIT_AMOUNT")); v != "" {
		amount, err := money.Parse(v)
		if err != nil {
			return Config{}, fmt.Errorf("%w: LEDGERCLOSE_MAX_UNIT_AMOUNT=%q: %v", ErrInvalid, v, err)
		}
		cfg.MaxUnitAmount = amount
	}
	if v := strings.TrimSpace(lookup("LEDGERCLOSE_TOKEN_TTL")); v != "" {
		ttl, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("%w: LEDGERCLOSE_TOKEN_TTL=%q", ErrInvalid, v)
		}
		cfg.TokenTTL = ttl
	}
	if v := strings.TrimSpace(lookup("LEDGERCLOSE_DELIVER_INSECURE")); v != "" {
		insecure, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("%w: LEDGERCLOSE_DELIVER_INSECURE=%q", ErrInvalid, v)
		}
		cfg.DeliverInsecure = insecure
	}
	cfg.TokenSecret = lookup("LEDGERCLOSE_TOKEN_SECRET")
	cfg.Renderer = strings.TrimSpace(lookup("LEDGERCLOSE_RENDERER"))
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func split(value string) []string {
	parts := make([]string, 0, 4)
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

// Validate refuses the configurations the service cannot run safely. The bounds are the hard
// ceilings of this package: a deployment that raised them would turn one request into an
// out-of-memory event, so the value is refused instead of accepted and clamped later.
func (c Config) Validate() error {
	switch {
	case strings.TrimSpace(c.Root) == "":
		return fmt.Errorf("%w: root", ErrMissing)
	case !strings.HasPrefix(c.Root, "/"):
		return fmt.Errorf("%w: root %q is not absolute", ErrInvalid, c.Root)
	case c.Root == "/":
		return fmt.Errorf("%w: the filesystem root is not a workspace", ErrInvalid)
	case len(c.TokenSecret) < 16:
		return fmt.Errorf("%w: token secret shorter than 16 bytes", ErrInvalid)
	case c.PageSize <= 0 || c.PageSize > HardPageSize:
		return fmt.Errorf("%w: page size %d is not in 1..%d", ErrInvalid, c.PageSize, HardPageSize)
	case c.MaxBatch <= 0 || c.MaxBatch > HardBatch:
		return fmt.Errorf("%w: max batch %d is not in 1..%d", ErrInvalid, c.MaxBatch, HardBatch)
	case c.MaxBody < 1024 || c.MaxBody > HardBody:
		return fmt.Errorf("%w: max body %d is not in 1024..%d", ErrInvalid, c.MaxBody, HardBody)
	case c.MaxEventQuantity <= 0 || c.MaxEventQuantity > HardQuantity:
		return fmt.Errorf("%w: max event quantity %d is not in 1..%d", ErrInvalid, c.MaxEventQuantity, HardQuantity)
	case c.MaxUnitAmount <= 0 || c.MaxUnitAmount > HardUnitAmount:
		return fmt.Errorf("%w: max unit amount %s is not in 0.01..%s", ErrInvalid, c.MaxUnitAmount, HardUnitAmount)
	case c.TokenTTL <= 0 || c.TokenTTL > 24*time.Hour:
		return fmt.Errorf("%w: token ttl %v is not in (0, 24h]", ErrInvalid, c.TokenTTL)
	case c.CloseWindow <= 0 || c.CloseWindow > 24:
		return fmt.Errorf("%w: close window %d is not in 1..24", ErrInvalid, c.CloseWindow)
	case c.TaxBasisPoints < 0 || c.TaxBasisPoints > 10_000:
		return fmt.Errorf("%w: tax basis points %d is not in 0..10000", ErrInvalid, c.TaxBasisPoints)
	case c.MaxResponse < 1024 || c.MaxResponse > HardResponse:
		return fmt.Errorf("%w: max response %d is not in 1024..%d", ErrInvalid, c.MaxResponse, HardResponse)
	case c.MaxLines <= 0 || c.MaxLines > HardLines:
		return fmt.Errorf("%w: max lines %d is not in 1..%d", ErrInvalid, c.MaxLines, HardLines)
	}
	seen := make(map[string]bool, len(c.Tenants))
	for _, tenant := range c.Tenants {
		if !ValidTenant(tenant) {
			return fmt.Errorf("%w: tenant %q is not a lowercase kebab id", ErrInvalid, tenant)
		}
		if seen[tenant] {
			return fmt.Errorf("%w: tenant %q is listed twice", ErrInvalid, tenant)
		}
		seen[tenant] = true
	}
	if len(c.Tenants) == 0 {
		return fmt.Errorf("%w: at least one tenant", ErrMissing)
	}
	return nil
}

// Limits reports the caps the handlers carry as a value.
func (c Config) Limits() usage.Limits {
	return usage.Limits{
		PageSize: c.PageSize, MaxBatch: c.MaxBatch, MaxBody: c.MaxBody,
		MaxEventQuantity: c.MaxEventQuantity, MaxUnitAmount: c.MaxUnitAmount,
		MaxLines: c.MaxLines, MaxResponse: c.MaxResponse, TaxBasisPoints: c.TaxBasisPoints,
	}
}

// TenantAllowed reports whether the deployment bills a tenant.
func (c Config) TenantAllowed(tenant string) bool {
	for _, known := range c.Tenants {
		if known == tenant {
			return true
		}
	}
	return false
}

// WindowAllows refuses a close that is not for a period inside the configured window. A close of a
// period that is still open would bill usage the month has not finished receiving, and a close of a
// period before the window would rewrite a statement already delivered.
func (c Config) WindowAllows(period calendar.Period, now time.Time) error {
	if !period.Valid() {
		return fmt.Errorf("%w: period %q", ErrInvalid, period)
	}
	age := calendar.MonthsBetween(period, calendar.Of(now))
	if age < 1 {
		return fmt.Errorf("%w: period %s is not over yet", ErrInvalid, period)
	}
	if age >= c.CloseWindow {
		return fmt.Errorf("%w: period %s is older than %d months", ErrInvalid, period, c.CloseWindow)
	}
	return nil
}
