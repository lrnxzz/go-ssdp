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
	// Search multicasts an M-SEARCH and yields every answer until maxWait, plus a grace period, passes.
	Search(ctx context.Context, target Target, maxWait time.Duration) iter.Seq2[Received[SearchResponse], error]

	// SearchUnicast asks a single device, at port 1900 or the SEARCHPORT.UPNP.ORG it announced.
	SearchUnicast(ctx context.Context, device netip.AddrPort, target Target) iter.Seq2[Received[SearchResponse], error]

	// Watch yields every change to the cache of devices announcing target until ctx ends.
	Watch(ctx context.Context, target Target) iter.Seq2[Event, error]
}

type ControlPointConfig struct {
	// Group is DefaultGroup when zero.
	Group Group

	UserAgent ProductTokens

	// Name and Uuid go out as CPFN.UPNP.ORG and CPUUID.UPNP.ORG.
	Name string
	Uuid string
}

func (c ControlPointConfig) Validate() error {
	if c.UserAgent.Upnp().AtLeast(Version20) && c.Name == "" {
		return fmt.Errorf("UDA %s requires a control point name", c.UserAgent.Upnp())
	}

	return nil
}

type controlPoint struct {
	config ControlPointConfig
}

func NewControlPoint(config ControlPointConfig) (ControlPoint, error) {
	config.Group = config.Group.orDefault()

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid control point configuration: %w", err)
	}

	return controlPoint{
		config: config,
	}, nil
}

func (c controlPoint) Search(ctx context.Context, target Target, maxWait time.Duration) iter.Seq2[Received[SearchResponse], error] {
	return func(yield func(Received[SearchResponse], error) bool) {
		packet, err := c.packet(c.config.Group.String(), target, maxWait)
		if err != nil {
			yield(Received[SearchResponse]{}, err)

			return
		}

		senders, err := openSenders(c.config.Group)
		if err != nil {
			yield(Received[SearchResponse]{}, err)

			return
		}

		searchCtx, cancel := context.WithTimeout(ctx, maxWait+searchGrace)
		defer cancel()

		c.run(ctx, searchCtx, senders, c.config.Group.Address(), packet, yield)
	}
}

func (c controlPoint) SearchUnicast(ctx context.Context, device netip.AddrPort, target Target) iter.Seq2[Received[SearchResponse], error] {
	return func(yield func(Received[SearchResponse], error) bool) {
		packet, err := c.packet(device.String(), target, 0)
		if err != nil {
			yield(Received[SearchResponse]{}, err)

			return
		}

		conn, err := net.ListenUDP(udpNetwork, nil)
		if err != nil {
			yield(Received[SearchResponse]{}, fmt.Errorf("failed to open a socket: %w", err))

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

func (c controlPoint) run(ctx, searchCtx context.Context, senders []endpoint, to netip.AddrPort, packet []byte, yield func(Received[SearchResponse], error) bool) {
	messages := listen(searchCtx, senders...)

	if err := send(searchCtx, senders, to, packet); err != nil {
		yield(Received[SearchResponse]{}, err)

		return
	}

	go repeat(searchCtx, senders, to, packet)

	for received := range messages {
		response, ok := received.Message.(SearchResponse)
		if !ok {

			slog.DebugContext(ctx, logIgnored, logKeyFrom, received.From, logKeyType, received.Message.Type())

			continue
		}

		answer := Received[SearchResponse]{
			Message:   response,
			From:      received.From,
			Interface: received.Interface,
		}

		if !yield(answer, nil) {
			return
		}
	}

	if err := ctx.Err(); err != nil {
		yield(Received[SearchResponse]{}, err)
	}
}

func (c controlPoint) Watch(ctx context.Context, target Target) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		packet, err := c.packet(c.config.Group.String(), target, MaxWaitFloor)
		if err != nil {
			yield(Event{}, err)

			return
		}

		listener, err := openListener(c.config.Group)
		if err != nil {
			yield(Event{}, err)

			return
		}

		senders, err := openSenders(c.config.Group)
		if err != nil {
			closeAll(listener)
			yield(Event{}, err)

			return
		}

		watchCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		messages := listen(watchCtx, append(slices.Clone(senders), listener)...)

		if err := send(watchCtx, senders, c.config.Group.Address(), packet); err != nil {
			yield(Event{}, err)

			return
		}

		go repeat(watchCtx, senders, c.config.Group.Address(), packet)

		watch(messages, newCache(target), yield)
	}
}

func watch(messages <-chan Received[Message], devices *cache, yield func(Event, error) bool) {
	expiry := time.NewTimer(0)
	expiry.Stop()

	defer expiry.Stop()

	for {
		var events []Event

		select {
		case received, ok := <-messages:
			if !ok {
				return
			}

			events = devices.apply(received, time.Now())
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

func (c controlPoint) packet(host string, target Target, maxWait time.Duration) ([]byte, error) {
	search := Search{
		Host:    host,
		Target:  target,
		MaxWait: maxWait,

		UserAgent:        c.config.UserAgent,
		ControlPointName: c.config.Name,
		ControlPointUuid: c.config.Uuid,
	}

	if err := search.Check(); err != nil {
		return nil, err
	}

	return search.MarshalBinary()
}
