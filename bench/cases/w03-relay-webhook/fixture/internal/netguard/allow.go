package netguard

import (
	"net"
	"strings"
)

// Allowlist is the set of hosts a deployment may call. Names are stored lowercased because a DNS
// name is case-insensitive; an address literal is stored as it parses.
type Allowlist struct {
	hosts map[string]bool
}

// NewAllowlist builds an allowlist from host names. An entry carrying a scheme, a path or a port is
// refused at construction rather than ignored: a malformed entry that silently disappeared would
// make a deployment call fewer hosts than its operator wrote down, or more.
func NewAllowlist(hosts []string) *Allowlist {
	list := &Allowlist{hosts: make(map[string]bool, len(hosts))}
	for _, raw := range hosts {
		host := strings.ToLower(strings.TrimSpace(raw))
		if host == "" || strings.ContainsAny(host, "/:@ ") {
			continue
		}
		list.hosts[host] = true
	}
	return list
}

// Has reports whether host is on the list. The comparison is over the parsed host with a trailing
// dot removed, so "hooks.example.com." and "hooks.example.com" are one name.
func (a *Allowlist) Has(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	return a.hosts[host]
}

// Len reports how many hosts the list carries.
func (a *Allowlist) Len() int { return len(a.hosts) }

// Entries lists the hosts in the order the map iteration produces, for a log line. A caller that
// needs a stable order sorts the result.
func (a *Allowlist) Entries() []string {
	out := make([]string, 0, len(a.hosts))
	for host := range a.hosts {
		out = append(out, host)
	}
	return out
}

// publicAddress reports whether ip is an address the service may call: a global unicast address
// that is not on a private, loopback, link-local or unspecified range.
func publicAddress(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	for _, block := range privateBlocks {
		if block.Contains(ip) {
			return false
		}
	}
	return true
}

// privateBlocks are the ranges a request from this service must never reach: the address space of
// the cluster the service runs in, and the metadata address every cloud hands out.
var privateBlocks = func() []*net.IPNet {
	cidrs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"100.64.0.0/10",
		"169.254.0.0/16",
		"fd00::/8",
		"::1/128",
	}
	blocks := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		if _, block, err := net.ParseCIDR(cidr); err == nil {
			blocks = append(blocks, block)
		}
	}
	return blocks
}()
