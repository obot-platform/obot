package safehttp

import (
	"net"
	"slices"
)

var (
	nat64WellKnown = mustParseCIDR("64:ff9b::/96")
	nat64LocalUse  = mustParseCIDR("64:ff9b:1::/48")
	sixToFour      = mustParseCIDR("2002::/16")
	teredo         = mustParseCIDR("2001::/32")
	ipv4Compatible = mustParseCIDR("::/96")
)

func mustParseCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

// IsLoopback reports whether ip is a loopback address, the unspecified address (which
// connects to the local host), or an IPv6 transition address that embeds one.
func IsLoopback(ip net.IP) bool {
	return anyIP(ip, func(ip net.IP) bool { return ip.IsLoopback() || ip.IsUnspecified() })
}

// IsPrivate reports whether ip is a private address or an IPv6 transition address that embeds one.
func IsPrivate(ip net.IP) bool {
	return anyIP(ip, net.IP.IsPrivate)
}

// IsLinkLocal reports whether ip is a link-local address or an IPv6 transition address that embeds one.
func IsLinkLocal(ip net.IP) bool {
	return anyIP(ip, func(ip net.IP) bool { return ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() })
}

func anyIP(ip net.IP, check func(net.IP) bool) bool {
	return check(ip) || slices.ContainsFunc(embeddedIPv4(ip), check)
}

// embeddedIPv4 returns the IPv4 addresses carried inside IPv6 transition addresses
// (NAT64, 6to4, Teredo and IPv4-compatible), which some networks route to the embedded address.
func embeddedIPv4(ip net.IP) []net.IP {
	if ip.To4() != nil {
		return nil
	}
	ip16 := ip.To16()
	if ip16 == nil {
		return nil
	}

	switch {
	case nat64WellKnown.Contains(ip16), nat64LocalUse.Contains(ip16):
		return []net.IP{net.IPv4(ip16[12], ip16[13], ip16[14], ip16[15])}
	case sixToFour.Contains(ip16):
		return []net.IP{net.IPv4(ip16[2], ip16[3], ip16[4], ip16[5])}
	case teredo.Contains(ip16):
		// Teredo carries the server address in bits 32-63 and the obfuscated client address in bits 96-127.
		return []net.IP{
			net.IPv4(ip16[4], ip16[5], ip16[6], ip16[7]),
			net.IPv4(^ip16[12], ^ip16[13], ^ip16[14], ^ip16[15]),
		}
	case ipv4Compatible.Contains(ip16):
		return []net.IP{net.IPv4(ip16[12], ip16[13], ip16[14], ip16[15])}
	}
	return nil
}
