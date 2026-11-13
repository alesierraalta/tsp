// Package config loads the service's configuration from the process environment and validates it before
// anything else is opened. Every value is read once, here, and a configuration that does not validate
// stops the process: a listener that starts with a page cap of zero or a secret of eight bytes is a
// listener an operator has to discover the hard way.
package config

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Keys are the environment variables the service reads. They are listed here so documentation and
// validation cannot drift from the loader.
const (
	KeyRoot      = "SHIPTRACK_ROOT"
	KeyAddr      = "SHIPTRACK_ADDR"
	KeyTenants   = "SHIPTRACK_TENANTS"
	KeySecret    = "SHIPTRACK_TOKEN_SECRET"
	KeyTokenTTL  = "SHIPTRACK_TOKEN_TTL"
	KeyZone      = "SHIPTRACK_ZONE"
	KeyPageSize  = "SHIPTRACK_PAGE_SIZE"
	KeyMaxBody   = "SHIPTRACK_MAX_BODY"
	KeyMaxBatch  = "SHIPTRACK_MAX_BATCH"
	KeyMaxGrams  = "SHIPTRACK_MAX_GRAMS"
	KeyMaxNote   = "SHIPTRACK_MAX_NOTE"
	KeyMaxLines  = "SHIPTRACK_MAX_LINES"
	KeyMaxResp   = "SHIPTRACK_MAX_RESPONSE"
	KeyMaxEvents = "SHIPTRACK_MAX_EVENTS"
	KeyMaxShips  = "SHIPTRACK_MAX_SHIPMENTS"
	KeyCloseWin  = "SHIPTRACK_CLOSE_WINDOW"
	KeyMaxCells  = "SHIPTRACK_MAX_EXPORT_CELLS"
	KeyTrailCap  = "SHIPTRACK_TRAIL_CAP"
	KeyTaxRate   = "SHIPTRACK_TAX_BASIS_POINTS"
	KeyEgress    = "SHIPTRACK_EGRESS_HOSTS"
	KeyInsecure  = "SHIPTRACK_DELIVER_INSECURE"
	KeyRenderer  = "SHIPTRACK_LABEL_RENDERER"
)

// Config is the validated configuration of one deployment.
type Config struct {
	Root        string
	Addr        string
	Tenants     []string
	TokenSecret []byte
	TokenTTL    time.Duration
	Zone        *time.Location
	ZoneName    string
	Caps        Caps
	TaxRate     int32
	EgressHosts []string
	Insecure    bool
	Renderer    string
}

// Load reads a configuration from an environment and validates it. The environment is a map rather than
// os.Getenv so a test states the deployment it means, and so a value is read once.
func Load(env Env) (*Config, error) {
	cfg := &Config{}
	var err error
	cfg.Root, err = requiredPath(env, KeyRoot)
	if err != nil {
		return nil, err
	}
	cfg.Addr = env.Get(KeyAddr)
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:8080"
	}
	tenants, err := env.CSV(KeyTenants)
	if err != nil {
		return nil, err
	}
	cfg.Tenants = tenants
	secret, err := env.Required(KeySecret)
	if err != nil {
		return nil, err
	}
	cfg.TokenSecret = []byte(secret)
	cfg.TokenTTL, err = env.Duration(KeyTokenTTL, time.Hour)
	if err != nil {
		return nil, err
	}
	cfg.ZoneName = env.Get(KeyZone)
	if cfg.ZoneName == "" {
		cfg.ZoneName = "UTC"
	}
	cfg.Zone, err = loadZone(cfg.ZoneName)
	if err != nil {
		return nil, err
	}
	cfg.Caps, err = ParseCaps(env)
	if err != nil {
		return nil, err
	}
	cfg.TaxRate, err = env.Int32(KeyTaxRate, 1900)
	if err != nil {
		return nil, err
	}
	hosts, err := env.CSV(KeyEgress)
	if err != nil {
		return nil, err
	}
	cfg.EgressHosts = hosts
	cfg.Insecure, err = env.Bool(KeyInsecure, false)
	if err != nil {
		return nil, err
	}
	cfg.Renderer = env.Get(KeyRenderer)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// ZoneOrDefault is the deployment's zone, or UTC when it has none.
func (c *Config) ZoneOrDefault() *time.Location {
	if c.Zone == nil {
		return time.UTC
	}
	return c.Zone
}

// TenantAllowed reports whether a tenant is one this deployment serves. Tenants are compared exactly:
// "acme" is served by a deployment configured for "acme" and not by one configured for "acme ".
func (c *Config) TenantAllowed(tenant string) bool {
	for _, allowed := range c.Tenants {
		if allowed == tenant {
			return true
		}
	}
	return false
}

// TenantsText renders the served tenants for a page or a log line.
func (c *Config) TenantsText() string { return strings.Join(c.Tenants, ",") }

// Variables renders the configuration table a status page shows: the key, whether it is set, and the
// value for the keys that are safe to print. A secret is never printed, only whether it is long enough.
func (c *Config) Variables() []string {
	lines := []string{
		KeyRoot + "=" + c.Root,
		KeyAddr + "=" + c.Addr,
		KeyTenants + "=" + c.TenantsText(),
		KeyTokenTTL + "=" + c.TokenTTL.String(),
		KeyZone + "=" + c.ZoneName,
		fmt.Sprintf("%s=%d", KeyPageSize, c.Caps.PageSize),
		fmt.Sprintf("%s=%d", KeyMaxBody, c.Caps.MaxBody),
		fmt.Sprintf("%s=%d", KeyMaxBatch, c.Caps.MaxBatch),
		fmt.Sprintf("%s=%d", KeyMaxGrams, c.Caps.MaxGrams),
		fmt.Sprintf("%s=%d", KeyMaxNote, c.Caps.MaxNote),
		fmt.Sprintf("%s=%d", KeyMaxLines, c.Caps.MaxLines),
		fmt.Sprintf("%s=%d", KeyMaxResp, c.Caps.MaxResponse),
		fmt.Sprintf("%s=%d", KeyMaxEvents, c.Caps.MaxEvents),
		fmt.Sprintf("%s=%d", KeyMaxShips, c.Caps.MaxShipments),
		fmt.Sprintf("%s=%d", KeyCloseWin, c.Caps.CloseWindowMonths),
		fmt.Sprintf("%s=%d", KeyMaxCells, c.Caps.MaxExportCells),
		fmt.Sprintf("%s=%d", KeyTrailCap, c.Caps.TrailCap),
		fmt.Sprintf("%s=%d", KeyTaxRate, c.TaxRate),
		KeyEgress + "=" + strings.Join(c.EgressHosts, ","),
		fmt.Sprintf("%s=%t", KeyInsecure, c.Insecure),
		KeyRenderer + "=" + c.Renderer,
	}
	sort.Strings(lines[5:])
	return lines
}

// loadZone resolves an IANA zone name. A name the platform does not carry is refused rather than
// replaced with UTC: a deployment that asked for a zone and silently got UTC would stamp the wrong day.
func loadZone(name string) (*time.Location, error) {
	if strings.TrimSpace(name) != name || name == "" {
		return nil, fmt.Errorf("config: zone %q", name)
	}
	if name == "UTC" {
		return time.UTC, nil
	}
	zone, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("config: zone %q: %w", name, err)
	}
	return zone, nil
}
