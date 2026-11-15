package netguard

import "net"

// Public reports whether an address is one an outbound request may reach: a global unicast address
// that is not on a private, loopback, link-local, multicast or unspecified range.
func Public(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, block := range privateBlocks {
		if block.Contains(ip) {
			return false
		}
	}
	return true
}

// PublicHost reports whether a host is an address a request may reach, resolving a name when it is not
// an address literal. It is the check the guard runs, exposed for a caller that has a host and no URL.
func PublicHost(host string, resolver Resolver) bool {
	if ip := net.ParseIP(host); ip != nil {
		return Public(ip)
	}
	if resolver == nil {
		resolver = LookupHost
	}
	addresses, err := resolver(host)
	if err != nil {
		return false
	}
	for _, address := range addresses {
		if !Public(address) {
			return false
		}
	}
	return len(addresses) > 0
}

// IsMetadata reports whether an address is the cloud metadata service, which is the range a delivery
// must never reach whatever the allowlist says.
func IsMetadata(ip net.IP) bool {
	return ip != nil && ip.Equal(net.ParseIP("169.254.169.254"))
}
