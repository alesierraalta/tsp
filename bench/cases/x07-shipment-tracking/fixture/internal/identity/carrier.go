package identity

import (
	"fmt"
	"strings"
)

// Resolve answers what a token's subject names. A subject is "partner:<id>" and the tenant comes from
// the token; both are compared byte for byte with the directory's entries, so a subject that differs
// in any byte — a capital, a trailing space, a Unicode look-alike — resolves to nothing. The tenant
// carried by the token is checked against the partner's own tenant as well: a token issued for tenant
// b cannot name partner p of tenant a even when it presents p's id.
func (d *Directory) Resolve(tenant, subject string) (Partner, error) {
	prefix, id, ok := strings.Cut(subject, ":")
	if !ok || !Same(prefix, "partner") {
		return Partner{}, fmt.Errorf("%w: subject %q", ErrInvalid, subject)
	}
	if !Same(strings.TrimSpace(subject), subject) {
		return Partner{}, fmt.Errorf("%w: subject %q carries whitespace", ErrInvalid, subject)
	}
	partner, err := d.Partner(tenant, id)
	if err != nil {
		return Partner{}, err
	}
	if !Same(partner.Tenant, tenant) {
		return Partner{}, fmt.Errorf("%w: %q is not a partner of tenant %q", ErrTenant, id, tenant)
	}
	return partner, nil
}

// Subject is the subject a partner's token carries.
func Subject(partner Partner) string { return "partner:" + partner.ID }

// TenantAllowed reports whether a tenant is one a deployment serves. Tenants are compared exactly,
// so a deployment configured for "acme" does not serve "acme ".
func TenantAllowed(tenant string, configured []string) bool {
	for _, allowed := range configured {
		if Same(allowed, tenant) {
			return true
		}
	}
	return false
}

// ResolveCarrier answers which carrier a scan event names. The carrier must be registered and must
// serve the shipment's lane, in this exact spelling; an event that names a carrier for a lane it does
// not serve is refused rather than accepted and reconciled later.
func (d *Directory) ResolveCarrier(id, origin, destination, service string) (Carrier, error) {
	carrier, err := d.Carrier(id)
	if err != nil {
		return Carrier{}, err
	}
	if !carrier.Serves(origin, destination, service) {
		return Carrier{}, fmt.Errorf("%w: carrier %q does not serve %s->%s %s", ErrNotFound, id, origin, destination, service)
	}
	return carrier, nil
}

// Describe renders a partner for a log line: id, tenant and name, with the name clipped so a stored
// display name cannot fill a log entry.
func (p Partner) Describe() string {
	name := p.Name
	if len(name) > 64 {
		name = name[:64]
	}
	return fmt.Sprintf("%s/%s (%s)", p.Tenant, p.ID, name)
}
