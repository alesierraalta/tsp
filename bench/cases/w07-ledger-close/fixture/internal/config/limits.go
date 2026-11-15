package config

import (
	"regexp"

	"example.com/ledgerclose/internal/money"
)

// The hard ceilings the service refuses to be configured above. They are constants rather than
// settings because a deployment that raised them turns one request into an out-of-memory event,
// and the number is not what a deployment needs to tune.
const (
	// HardPageSize is the largest page any list endpoint may return.
	HardPageSize = 500
	// HardBatch is the largest number of events one ingest request may carry.
	HardBatch = 5_000
	// HardBody is the largest request body the service decodes.
	HardBody = 4 << 20
	// HardQuantity is the largest quantity one usage event may carry.
	HardQuantity = 1_000_000_000
	// HardResponse is the largest response body a delivery reads back.
	HardResponse = 1 << 20
	// HardLines is the largest number of lines one invoice may carry.
	HardLines = 20_000
)

// HardUnitAmount is the largest unit amount one usage event may carry.
const HardUnitAmount = money.Amount(1_000_000_00)

// tenantRE is the form of a tenant id. The id decides a directory name under the workspace root,
// so it only accepts the lowercase kebab form: a tenant id is never a path.
var tenantRE = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}[a-z0-9]$`)

// ValidTenant reports whether a tenant id is one the service accepts.
func ValidTenant(tenant string) bool { return tenantRE.MatchString(tenant) }
