package netguard

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

// Classification is what a deployment may say about an address.
type Classification string

// The classifications the guard reports.
const (
	Public    Classification = "public"
	Loopback  Classification = "loopback"
	Private   Classification = "private"
	LinkLocal Classification = "link-local"
	Metadata  Classification = "metadata"
	Other     Classification = "other"
)

// metadataAddresses are the addresses cloud instances answer on. A request to one of them reads the
// instance's credentials, which is why they are classified apart from the private ranges.
var metadataAddresses = func() []*net.IPNet {
	blocks := make([]*net.IPNet, 0, 2)
	for _, cidr := range []string{"169.254.169.254/32", "fd00:ec2::254/128"} {
		if _, block, err := net.ParseCIDR(cidr); err == nil {
			blocks = append(blocks, block)
		}
	}
	return blocks
}()

// Classify names the range an address belongs to. A host name is reported as "other": the guard
// decides names against the allowlist, and this function classifies addresses.
func Classify(host string) Classification {
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return Other
	}
	for _, block := range metadataAddresses {
		if block.Contains(ip) {
			return Metadata
		}
	}
	if ip.IsLoopback() {
		return Loopback
	}
	if ip.IsLinkLocalUnicast() {
		return LinkLocal
	}
	for _, block := range privateBlocks {
		if block.Contains(ip) {
			return Private
		}
	}
	if !publicAddress(ip) {
		return Other
	}
	return Public
}

// Describe renders the policy a deployment runs with, which is what the operator endpoint prints:
// the allowlist and the ranges that are never called.
func Describe(guard *Guard) map[string]any {
	entries := guard.Allowlist().Entries()
	sort.Strings(entries)
	return map[string]any{
		"allowlist": entries,
		"refused_ranges": []string{
			string(Loopback), string(Private), string(LinkLocal), string(Metadata),
		},
		"schemes": []string{"https"},
		"port":    443,
	}
}

// Addresses renders the private blocks as CIDR strings, for a deployment that wants to print them.
func Addresses() []string {
	out := make([]string, 0, len(privateBlocks))
	for _, block := range privateBlocks {
		out = append(out, block.String())
	}
	sort.Strings(out)
	return out
}

// ParseHost reduces an endpoint's host part to the form the allowlist compares: no port, no
// brackets, no trailing dot.
func ParseHost(raw string) (string, error) {
	host := strings.TrimSpace(raw)
	if host == "" {
		return "", fmt.Errorf("%w: empty host", ErrMalformed)
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "" {
		return "", fmt.Errorf("%w: %q has no host", ErrMalformed, raw)
	}
	return host, nil
}
