package quota

import (
	"errors"
	"strings"
	"time"
)

var ErrFull = errors.New("tenant quota exhausted")
var ErrTenant = errors.New("tenant must be a nonempty identifier")

// Config is identical for gateways sharing a namespace.
type Config struct {
	Namespace string
	Limit     int
	Lease     time.Duration
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Namespace) == "" {
		return errors.New("namespace is required")
	}
	if c.Limit < 1 {
		return errors.New("limit must be positive")
	}
	if c.Lease <= 0 {
		return errors.New("lease must be positive")
	}
	return nil
}

func tenantKey(namespace, tenant string) (string, error) {
	if strings.TrimSpace(tenant) == "" || strings.ContainsAny(tenant, "\n\r\x00") {
		return "", ErrTenant
	}
	return namespace + ":" + tenant, nil
}
