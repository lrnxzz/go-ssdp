package ssdp

import (
	"fmt"
	"net/http"
	"net/netip"
	"time"
)

// Search is an M-SEARCH: multicast with MX, or unicast to one device without it.
type Search struct {
	Host    string
	Target  Target
	MaxWait time.Duration

	UserAgent        ProductTokens
	ControlPointName string
	ControlPointUuid string
	TcpPort          Optional[int]

	Extensions http.Header
}

func (s Search) Type() MessageType {
	return MessageSearch
}

// IsMulticast reports whether HOST names a multicast group; a missing port means 1900.
func (s Search) IsMulticast() bool {
	address, ok := hostAddress(s.Host)

	return ok && address.IsMulticast()
}

func (s Search) MarshalBinary() ([]byte, error) {
	if s.Host == "" || s.Target.IsZero() || s.MaxWait < 0 || s.MaxWait%time.Second != 0 {
		return nil, fmt.Errorf("%w: a search needs HOST, ST and MX in whole seconds", ErrMalformed)
	}

	return newWriter().
		requestLine(MethodSearch).
		field(HeaderHost, s.Host).
		field(HeaderMan, ManDiscover).
		seconds(HeaderMaxWait, s.MaxWait).
		field(HeaderSearchTarget, s.Target.String()).
		optional(HeaderUserAgent, s.UserAgent.String()).
		optional(HeaderControlPointName, s.ControlPointName).
		optional(HeaderControlPointUuid, s.ControlPointUuid).
		number(HeaderTcpPort, s.TcpPort).
		extensions(s.Extensions).
		bytes(), nil
}

func (s *Search) UnmarshalBinary(data []byte) error {
	return unmarshal(data, s)
}

func (s *Search) read(f frame) error {
	if man := f.header.Get(HeaderMan); man != ManDiscover {
		return fmt.Errorf("%w: MAN %q is not %s", ErrMalformed, man, ManDiscover)
	}

	target, err := f.target(HeaderSearchTarget)
	if err != nil {
		return err
	}

	maxWait, err := f.seconds(HeaderMaxWait)
	if err != nil {
		return err
	}

	tcpPort, err := f.port(HeaderTcpPort)
	if err != nil {
		return err
	}

	*s = Search{
		Host:    f.header.Get(HeaderHost),
		Target:  target,
		MaxWait: maxWait,

		UserAgent:        f.productTokens(HeaderUserAgent),
		ControlPointName: f.header.Get(HeaderControlPointName),
		ControlPointUuid: f.header.Get(HeaderControlPointUuid),
		TcpPort:          tcpPort,

		Extensions: f.extensions(),
	}

	return nil
}

func (s Search) Check() error {
	violations := make([]error, 0)

	if s.Host == "" {
		violations = append(violations, violation("%s is required", HeaderHost))
	}

	if s.Target.IsZero() {
		violations = append(violations, violation("%s is required", HeaderSearchTarget))
	}

	if s.IsMulticast() && s.MaxWait < MaxWaitFloor {
		violations = append(violations, violation("a multicast search requires %s of at least %s", HeaderMaxWait, MaxWaitFloor))
	}

	violations = append(violations,
		requiredSince(s.UserAgent.Upnp().rules(), Version20, s.ControlPointName != "", HeaderControlPointName),
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
