package ssdp

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
	"net"
	"net/netip"
	"slices"
	"time"
)

// ControlPoint searches the network for devices and keeps track of them.
type ControlPoint interface {
	// Search multicasts an M-SEARCH and yields each answer once until maxWait, plus a grace period, passes.
	Search(ctx context.Context, target string, maxWait time.Duration) iter.Seq2[SearchResponse, error]

	// SearchUnicast asks a single device, at port 1900 or the SEARCHPORT.UPNP.ORG it announced.
	SearchUnicast(ctx context.Context, device netip.AddrPort, target string) iter.Seq2[SearchResponse, error]

	// Watch yields every change to the cache of devices announcing target until ctx ends.
	Watch(ctx context.Context, target string) iter.Seq2[Event, error]
}

type ControlPointConfig struct {
	// Group is DefaultGroup when zero.
	Group Group

	// UserAgent is built from the platform when empty.
	UserAgent string

	// Name and Uuid go out as CPFN.UPNP.ORG and CPUUID.UPNP.ORG.
	Name string
	Uuid string

	// Logger is slog.Default() when nil.
	Logger *slog.Logger
}

func (c ControlPointConfig) Validate() error {
	if c.UserAgent == "" {
		return nil
	}

	userAgent, err := ParseProductTokens(c.UserAgent)
	if err != nil {
		return fmt.Errorf("invalid user agent: %w", err)
	}

	if userAgent.Upnp().AtLeast(Version20) && c.Name == "" {
		return fmt.Errorf("UDA %s requires a control point name", userAgent.Upnp())
	}

	return nil
}

type controlPoint struct {
	transport transport
	userAgent ProductTokens
	name      string
	uuid      string
}

func NewControlPoint(config ControlPointConfig) (ControlPoint, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid control point configuration: %w", err)
	}

	return newControlPoint(config), nil
}

func newControlPoint(config ControlPointConfig) controlPoint {
	userAgent := defaultProductTokens()
	if config.UserAgent != "" {
		userAgent = readProductTokens(config.UserAgent)
	}

	return controlPoint{
		transport: newTransport(config.Group, config.Logger),
		userAgent: userAgent,
		name:      config.Name,
		uuid:      config.Uuid,
	}
}

// Search searches from a control point with the default configuration.
func Search(ctx context.Context, target string, maxWait time.Duration) iter.Seq2[SearchResponse, error] {
	return newControlPoint(ControlPointConfig{}).Search(ctx, target, maxWait)
}

// Watch watches from a control point with the default configuration.
func Watch(ctx context.Context, target string) iter.Seq2[Event, error] {
	return newControlPoint(ControlPointConfig{}).Watch(ctx, target)
}

func (c controlPoint) Search(ctx context.Context, target string, maxWait time.Duration) iter.Seq2[SearchResponse, error] {
	return func(yield func(SearchResponse, error) bool) {
		wait := min(max((maxWait+time.Second-1).Truncate(time.Second), maxWaitFloor), maxWaitCeiling)

		packet, err := c.packet(c.transport.group.String(), target, wait)
		if err != nil {
			yield(SearchResponse{}, err)

			return
		}

		senders, err := c.transport.openSenders()
		if err != nil {
			yield(SearchResponse{}, err)

			return
		}

		searchCtx, cancel := context.WithTimeout(ctx, wait+searchGrace)
		defer cancel()

		c.run(ctx, searchCtx, senders, c.transport.group.Address(), packet, yield)
	}
}

func (c controlPoint) SearchUnicast(ctx context.Context, device netip.AddrPort, target string) iter.Seq2[SearchResponse, error] {
	return func(yield func(SearchResponse, error) bool) {
		packet, err := c.packet(device.String(), target, 0)
		if err != nil {
			yield(SearchResponse{}, err)

			return
		}

		conn, err := net.ListenUDP(udpNetwork, nil)
		if err != nil {
			yield(SearchResponse{}, fmt.Errorf("failed to open a socket: %w", err))

			return
		}

		senders := []endpoint{
			newEndpoint(conn, defaultInterface),
		}

		searchCtx, cancel := context.WithTimeout(ctx, unicastSearchWait)
		defer cancel()

		c.run(ctx, searchCtx, senders, device, packet, yield)
	}
}

func (c controlPoint) run(ctx, searchCtx context.Context, senders []endpoint, to netip.AddrPort, packet []byte, yield func(SearchResponse, error) bool) {
	messages := c.transport.listen(searchCtx, senders...)

	if err := c.transport.send(searchCtx, senders, to, packet); err != nil {
		yield(SearchResponse{}, err)

		return
	}

	go c.transport.repeat(searchCtx, senders, to, packet)

	// The M-SEARCH goes out more than once (§1.3.2), and devices answer every copy.
	seen := make(map[string]bool)

	for datagram := range messages {
		response, ok := datagram.message.(SearchResponse)
		if !ok {
			c.transport.logger.DebugContext(ctx, "ssdp: message ignored", "from", datagram.from, "type", datagram.message.Type())

			continue
		}

		if seen[response.ServiceName.String()] {
			continue
		}

		seen[response.ServiceName.String()] = true

		response.From = datagram.from
		response.Interface = datagram.interfaceName

		if !yield(response, nil) {
			return
		}
	}

	if err := ctx.Err(); err != nil {
		yield(SearchResponse{}, err)
	}
}

func (c controlPoint) Watch(ctx context.Context, target string) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		watched, err := ParseTarget(target)
		if err != nil {
			yield(Event{}, err)

			return
		}

		packet, err := c.packet(c.transport.group.String(), target, maxWaitFloor)
		if err != nil {
			yield(Event{}, err)

			return
		}

		listener, err := c.transport.openListener()
		if err != nil {
			yield(Event{}, err)

			return
		}

		senders, err := c.transport.openSenders()
		if err != nil {
			closeAll(listener)
			yield(Event{}, err)

			return
		}

		watchCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		messages := c.transport.listen(watchCtx, append(slices.Clone(senders), listener)...)

		if err := c.transport.send(watchCtx, senders, c.transport.group.Address(), packet); err != nil {
			yield(Event{}, err)

			return
		}

		go c.transport.repeat(watchCtx, senders, c.transport.group.Address(), packet)

		watch(messages, newCache(watched), yield)
	}
}

func watch(messages <-chan received, devices *cache, yield func(Event, error) bool) {
	expiry := time.NewTimer(0)
	expiry.Stop()

	defer expiry.Stop()

	for {
		var events []Event

		select {
		case datagram, ok := <-messages:
			if !ok {
				return
			}

			events = devices.apply(datagram, time.Now())
		case now := <-expiry.C:
			events = devices.expire(now)
		}

		for _, event := range events {
			if !yield(event, nil) {
				return
			}
		}

		if next, ok := devices.nextExpiry(); ok {
			expiry.Reset(time.Until(next))
		} else {
			expiry.Stop()
		}
	}
}

func (c controlPoint) packet(host, target string, maxWait time.Duration) ([]byte, error) {
	parsed, err := ParseTarget(target)
	if err != nil {
		return nil, err
	}

	request := SearchRequest{
		Host:    host,
		Target:  parsed,
		MaxWait: maxWait,

		UserAgent:        c.userAgent,
		ControlPointName: c.name,
		ControlPointUuid: c.uuid,
	}

	if err := request.Check(); err != nil {
		return nil, err
	}

	return request.MarshalBinary()
}
