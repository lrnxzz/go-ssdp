package ssdp

import (
	"fmt"
	"net/http"
	"net/netip"
	"time"
)

// SearchRequest is an M-SEARCH: multicast with MX, or unicast to one device without it.
type SearchRequest struct {
	Host    string
	Target  Target
	MaxWait time.Duration

	UserAgent        ProductTokens
	ControlPointName string
	ControlPointUuid string
	TcpPort          Optional[int]

	Extensions http.Header
}

func (s SearchRequest) Type() MessageType {
	return MessageSearchRequest
}

// IsMulticast reports whether HOST names a multicast group; a missing port means 1900.
func (s SearchRequest) IsMulticast() bool {
	address, ok := hostAddress(s.Host)

	return ok && address.IsMulticast()
}

func (s SearchRequest) MarshalBinary() ([]byte, error) {
	if s.Host == "" || s.Target.IsZero() || s.MaxWait < 0 || s.MaxWait%time.Second != 0 {
		return nil, fmt.Errorf("%w: a search needs HOST, ST and MX in whole seconds", ErrMalformed)
	}

	return newWriter().
		requestLine(methodSearch).
		field(headerHost, s.Host).
		field(headerMan, manDiscover).
		seconds(headerMaxWait, s.MaxWait).
		field(headerSearchTarget, s.Target.String()).
		optional(headerUserAgent, s.UserAgent.String()).
		optional(headerControlPointName, s.ControlPointName).
		optional(headerControlPointUuid, s.ControlPointUuid).
		number(headerTcpPort, s.TcpPort).
		extensions(s.Extensions).
		bytes(), nil
}

func (s *SearchRequest) UnmarshalBinary(data []byte) error {
	return unmarshal(data, s)
}

func (s *SearchRequest) read(f frame) error {
	if man := f.header.Get(headerMan); man != manDiscover {
		return fmt.Errorf("%w: MAN %q is not %s", ErrMalformed, man, manDiscover)
	}

	target, err := f.target(headerSearchTarget)
	if err != nil {
		return err
	}

	maxWait, err := f.seconds(headerMaxWait)
	if err != nil {
		return err
	}

	tcpPort, err := f.port(headerTcpPort)
	if err != nil {
		return err
	}

	*s = SearchRequest{
		Host:    f.header.Get(headerHost),
		Target:  target,
		MaxWait: maxWait,

		UserAgent:        f.productTokens(headerUserAgent),
		ControlPointName: f.header.Get(headerControlPointName),
		ControlPointUuid: f.header.Get(headerControlPointUuid),
		TcpPort:          tcpPort,

		Extensions: f.extensions(),
	}

	return nil
}

func (s SearchRequest) Check() error {
	violations := make([]error, 0)

	if s.Host == "" {
		violations = append(violations, violation("%s is required", headerHost))
	}

	if s.Target.IsZero() {
		violations = append(violations, violation("%s is required", headerSearchTarget))
	}

	if s.IsMulticast() && s.MaxWait < maxWaitFloor {
		violations = append(violations, violation("a multicast search requires %s of at least %s", headerMaxWait, maxWaitFloor))
	}

	violations = append(violations,
		requiredSince(s.UserAgent.Upnp().rules(), Version20, s.ControlPointName != "", headerControlPointName),
	)

	return violationsOf(violations)
}

func hostAddress(host string) (netip.Addr, bool) {
	if address, err := netip.ParseAddrPort(host); err == nil {
		return address.Addr(), true
	}

	address, err := netip.ParseAddr(host)

	return address, err == nil
}
