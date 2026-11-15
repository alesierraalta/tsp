package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"example.com/archive/internal/clock"
)

// Validate refuses a configuration the service cannot run with. Every refusal names the value, so an
// operator reads the startup failure and knows which variable to change.
func (c Config) Validate() error {
	var problems []string
	switch {
	case strings.TrimSpace(c.Root) == "":
		problems = append(problems, "root is not configured")
	case !filepath.IsAbs(c.Root):
		problems = append(problems, fmt.Sprintf("root %q is not absolute", c.Root))
	case filepath.Clean(c.Root) == string(filepath.Separator):
		problems = append(problems, "root is the filesystem root")
	}
	if strings.TrimSpace(c.Addr) == "" {
		problems = append(problems, "listen address is not configured")
	}
	if len(c.Tenants) == 0 {
		problems = append(problems, "no tenants")
	}
	for i, t := range c.Tenants {
		if strings.TrimSpace(t) != t || t == "" {
			problems = append(problems, fmt.Sprintf("tenant %d is not a trimmed id", i))
		}
		if strings.ContainsAny(t, "/\\,") {
			problems = append(problems, fmt.Sprintf("tenant %q carries a separator", t))
		}
	}
	if c.PageSize <= 0 {
		problems = append(problems, "page size must be positive")
	}
	if len(c.TokenSecret) < 16 {
		problems = append(problems, "token secret is shorter than 16 bytes")
	}
	if c.SessionTTL <= 0 {
		problems = append(problems, "session lifetime must be positive")
	}
	if c.RetentionDays <= 0 {
		problems = append(problems, "retention window must be positive")
	}
	if _, err := clock.LoadZone(c.Zone); err != nil {
		problems = append(problems, err.Error())
	}
	if err := c.Limits().Check(); err != nil {
		problems = append(problems, err.Error())
	}
	if len(problems) > 0 {
		return fmt.Errorf("config: %s", strings.Join(problems, "; "))
	}
	return nil
}

// ParseTTL reads a duration a deployment wrote in the environment: 45m, 2h, 30s. A bare number is
// refused, because "60" could mean seconds or minutes and a session lifetime cannot be guessed.
func ParseTTL(s string) (time.Duration, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return 0, fmt.Errorf("config: empty duration")
	}
	d, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, fmt.Errorf("config: duration %q: %w", s, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("config: duration %q is not positive", s)
	}
	return d, nil
}

// ReportingZone is the zone the operator pages print instants in. The value is loaded once at
// startup; the stored instants and every retention comparison stay UTC.
func (c Config) ReportingZone() *time.Location {
	loc, err := clock.LoadZone(c.Zone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// RetentionHorizon is the oldest UTC day a closed batch may carry and still be kept. A sweep
// compares batch days to it; a batch that closed today is inside the window whatever the hour.
func (c Config) RetentionHorizon(now time.Time) time.Time {
	return clock.AddDays(now, -c.RetentionDays)
}

// StoreDirs are the directories the service creates under the root, in creation order. A deployment
// that pre-creates them itself still has the same tree.
func (c Config) StoreDirs() []string {
	return []string{"objects", "objects/blobs", "objects/meta", "objects/index", ".staging", "audit"}
}

// MaxPagesForLimit caps a declared page count at the deployment's page cap. The helper exists so a
// caller reading a page count has one way to clamp it; a caller that does not clamp it allocates for
// whatever the manifest asked.
func (c Config) MaxPagesForLimit(declared int) int {
	if declared > c.MaxPages {
		return c.MaxPages
	}
	if declared < 0 {
		return 0
	}
	return declared
}
