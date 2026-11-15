// Package identity is the directory of the tenants, partners and carriers the service is allowed to
// act for, and the rule that decides whether two identifiers name the same party. The rule is exact:
// two ids are the same party when their bytes are equal, with no case folding, no whitespace trimming
// and no Unicode normalization. `Partner` and `partner` are two partners, and `\u212Aelvin` and
// `Kelvin` are two subjects, which is what makes a token's subject check a check and not a guess.
package identity

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Errors the directory reports.
var (
	ErrInvalid  = errors.New("identity: invalid")
	ErrNotFound = errors.New("identity: not found")
	ErrTenant   = errors.New("identity: tenant mismatch")
)

// Partner is a merchant the service bills and ships for. ID is a name segment; Name is display text
// and is escaped where it is rendered, never used to build a path. Endpoint is the partner's notification
// URL: the service reads it from the partner record, never from the request that asks for a delivery.
type Partner struct {
	Tenant   string
	ID       string
	Name     string
	Zone     string
	Endpoint string
}

// Carrier is a carrier the service assigns shipments to. A carrier carries lanes; a lane is a pair of
// facility codes the carrier serves.
type Carrier struct {
	ID    string
	Name  string
	Lanes []Lane
}

// Lane is one origin/destination pair a carrier serves, with the service levels it offers.
type Lane struct {
	Origin      string
	Destination string
	Services    []string
}

// Same compares two identifiers exactly. It is a byte comparison and nothing else: the service never
// lowercases an id to make it "match", because that would let a caller who cannot present the id
// present a variant of it.
func Same(a, b string) bool { return a == b }

// Canonical reports whether an id has the form the directory accepts: a lower-case name segment of 2
// to 32 bytes. It is a form rule; membership in the directory is a separate lookup.
func Canonical(id string) bool {
	if len(id) < 2 || len(id) > 32 {
		return false
	}
	if id[0] < 'a' || id[0] > 'z' {
		return false
	}
	last := id[len(id)-1]
	if (last < 'a' || last > 'z') && (last < '0' || last > '9') {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

// Directory holds the partners and carriers, keyed by tenant and id. Reads are safe from several
// goroutines; a write takes the lock for itself.
type Directory struct {
	mu       sync.RWMutex
	partners map[string]Partner
	carriers map[string]Carrier
}

// NewDirectory returns an empty directory.
func NewDirectory() *Directory {
	return &Directory{partners: map[string]Partner{}, carriers: map[string]Carrier{}}
}

func partnerKey(tenant, id string) string { return tenant + "\x00" + id }

// AddPartner registers a partner. The tenant and the id are validated here rather than by the caller,
// because a partner id becomes a statement-directory name.
func (d *Directory) AddPartner(p Partner) (Partner, error) {
	if !Canonical(p.Tenant) || !Canonical(p.ID) {
		return Partner{}, fmt.Errorf("%w: partner %q of tenant %q", ErrInvalid, p.ID, p.Tenant)
	}
	if strings.TrimSpace(p.Name) == "" {
		return Partner{}, fmt.Errorf("%w: partner %q has no name", ErrInvalid, p.ID)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	key := partnerKey(p.Tenant, p.ID)
	if _, exists := d.partners[key]; exists {
		return Partner{}, fmt.Errorf("%w: partner %q of tenant %q is already registered", ErrInvalid, p.ID, p.Tenant)
	}
	d.partners[key] = p
	return p, nil
}

// Partner resolves a partner of a tenant exactly. A partner of another tenant is not found: the
// lookup is keyed by both, so a token that names tenant b cannot read tenant a's partner by id.
func (d *Directory) Partner(tenant, id string) (Partner, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	p, ok := d.partners[partnerKey(tenant, id)]
	if !ok {
		return Partner{}, fmt.Errorf("%w: partner %q of tenant %q", ErrNotFound, id, tenant)
	}
	return p, nil
}

// PartnerAnyTenant resolves a partner id without a tenant. It is used by the close, which iterates the
// tenants it is configured for and calls Partner with each; nothing on a request path uses it.
func (d *Directory) PartnerAnyTenant(id string) (Partner, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var found []Partner
	for _, p := range d.partners {
		if Same(p.ID, id) {
			found = append(found, p)
		}
	}
	if len(found) != 1 {
		return Partner{}, fmt.Errorf("%w: partner %q matches %d entries", ErrNotFound, id, len(found))
	}
	return found[0], nil
}

// Partners lists the partners of a tenant, sorted by id.
func (d *Directory) Partners(tenant string) []Partner {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var list []Partner
	for _, p := range d.partners {
		if Same(p.Tenant, tenant) {
			list = append(list, p)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list
}

// AddCarrier registers a carrier. Its lanes are checked so a lane code cannot be empty or carry a
// separator: the code is displayed and matched, never joined into a path.
func (d *Directory) AddCarrier(c Carrier) (Carrier, error) {
	if !Canonical(c.ID) {
		return Carrier{}, fmt.Errorf("%w: carrier %q", ErrInvalid, c.ID)
	}
	if strings.TrimSpace(c.Name) == "" {
		return Carrier{}, fmt.Errorf("%w: carrier %q has no name", ErrInvalid, c.ID)
	}
	for i, lane := range c.Lanes {
		if !Canonical(lane.Origin) || !Canonical(lane.Destination) || len(lane.Services) == 0 {
			return Carrier{}, fmt.Errorf("%w: carrier %q lane %d", ErrInvalid, c.ID, i)
		}
		for _, service := range lane.Services {
			if !Canonical(service) {
				return Carrier{}, fmt.Errorf("%w: carrier %q service %q", ErrInvalid, c.ID, service)
			}
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.carriers[c.ID]; exists {
		return Carrier{}, fmt.Errorf("%w: carrier %q is already registered", ErrInvalid, c.ID)
	}
	d.carriers[c.ID] = c
	return c, nil
}

// Carrier resolves a carrier by id, exactly.
func (d *Directory) Carrier(id string) (Carrier, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	c, ok := d.carriers[id]
	if !ok {
		return Carrier{}, fmt.Errorf("%w: carrier %q", ErrNotFound, id)
	}
	return c, nil
}

// Carriers lists the registered carriers, sorted by id.
func (d *Directory) Carriers() []Carrier {
	d.mu.RLock()
	defer d.mu.RUnlock()
	list := make([]Carrier, 0, len(d.carriers))
	for _, c := range d.carriers {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list
}

// Serves reports whether a carrier serves a lane with a service, exactly. A carrier that serves
// "ground" does not serve "Ground".
func (c Carrier) Serves(origin, destination, service string) bool {
	for _, lane := range c.Lanes {
		if !Same(lane.Origin, origin) || !Same(lane.Destination, destination) {
			continue
		}
		for _, offered := range lane.Services {
			if Same(offered, service) {
				return true
			}
		}
	}
	return false
}
