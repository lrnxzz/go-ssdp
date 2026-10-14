package ssdp

import (
	"fmt"
	"net/netip"
)

const highestTtl = 255

// DefaultGroup is 239.255.255.250:1900 with a TTL of 2.
var DefaultGroup = Group{
	address: netip.MustParseAddrPort(MulticastAddress),
	ttl:     DefaultTtl,
}

// Group is the multicast group SSDP runs on and the TTL its datagrams leave with.
type Group struct {
	address netip.AddrPort
	ttl     int
}

func NewGroup(address string, ttl int) (Group, error) {
	parsed, err := netip.ParseAddrPort(address)
	if err != nil {
		return Group{}, fmt.Errorf("invalid group address %q: %w", address, err)
	}

	if !parsed.Addr().Is4() || !parsed.Addr().IsMulticast() || parsed.Port() == 0 {
		return Group{}, fmt.Errorf("group address %s is not an IPv4 multicast address with a port", parsed)
	}

	if ttl < 0 || ttl > highestTtl {
		return Group{}, fmt.Errorf("ttl %d is outside 0-%d", ttl, highestTtl)
	}

	return Group{
		address: parsed,
		ttl:     ttl,
	}, nil
}

func (g Group) Address() netip.AddrPort {
	return g.address
}

func (g Group) Ttl() int {
	return g.ttl
}

func (g Group) IsZero() bool {
	return !g.address.IsValid()
}

func (g Group) String() string {
	return g.address.String()
}

func (g Group) orDefault() Group {
	if g.IsZero() {
		return DefaultGroup
	}

	return g
}
