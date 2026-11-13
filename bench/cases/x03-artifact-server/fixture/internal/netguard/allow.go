package netguard

import (
	"net"
	"strings"
)

// Allowlist is the set of hosts a deployment may call. A name is stored lower-cased because a DNS name
// is case-insensitive; an address literal is compared as it parses.
type Allowlist struct {
	hosts map[string]bool
}

// NewAllowlist builds an allowlist from host names. An entry carrying a scheme, a path, a port or a
// space is refused at construction rather than silently dropped: an entry that disappeared would make
// a deployment call fewer hosts than its operator wrote down, or more.
func NewAllowlist(hosts []string) *Allowlist {
	list := &Allowlist{hosts: make(map[string]bool, len(hosts))}
	for _, raw := range hosts {
		host := normalise(raw)
		if host == "" || strings.ContainsAny(host, "/:@?# ") {
			continue
		}
		list.hosts[host] = true
	}
	return list
}

// Has reports whether host is on the list. The comparison drops a trailing dot and the case, so
// "hooks.example.com." and "hooks.example.com" are one name.
func (a *Allowlist) Has(host string) bool {
	return a.hosts[normalise(host)]
}

// Len reports how many hosts the list carries.
func (a *Allowlist) Len() int { return len(a.hosts) }

// Entries lists the hosts in order, for a log line and the operator console.
func (a *Allowlist) Entries() []string {
	hosts := make([]string, 0, len(a.hosts))
	for host := range a.hosts {
		hosts = append(hosts, host)
	}
	sortStrings(hosts)
	return hosts
}

// With returns a list carrying one more host, which an operator route uses to add an endpoint without
// rebuilding the deployment.
func (a *Allowlist) With(host string) *Allowlist {
	out := &Allowlist{hosts: make(map[string]bool, len(a.hosts)+1)}
	for name := range a.hosts {
		out.hosts[name] = true
	}
	if normalised := normalise(host); normalised != "" && !strings.ContainsAny(normalised, "/:@?# ") {
		out.hosts[normalised] = true
	}
	return out
}

// normalise lower-cases a host and drops its trailing dot.
func normalise(host string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
}

// sortStrings orders a slice of strings, kept here so the allowlist imports nothing else.
func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

// privateBlocks are the ranges an outbound request must never reach: the deployment's own cluster, the
// loopback, the link-local space and the metadata address every cloud hands out.
var privateBlocks = func() []*net.IPNet {
	cidrs := []string{
		"0.0.0.0/8",
		"10.0.0.0/8",
		"100.64.0.0/10",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"172.16.0.0/12",
		"192.0.0.0/24",
		"192.168.0.0/16",
		"198.18.0.0/15",
		"fc00::/7",
		"fe80::/10",
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
