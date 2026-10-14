package ssdp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/netip"
	"net/url"
	"sync"
	"time"
)

// Device announces itself and answers searches until it closes.
type Device interface {
	// Close sends ssdp:byebye for every advertisement and stops answering.
	Close() error
}

// Advertisement is one NT and USN pair a device announces and answers searches for.
type Advertisement struct {
	Target      Target
	ServiceName ServiceName
}

func NewAdvertisement(uuid string, target Target) Advertisement {
	return Advertisement{
		Target:      target,
		ServiceName: NewServiceName(uuid, target),
	}
}

// NewRootDeviceAdvertisements is the set a root device announces: upnp:rootdevice, its uuid, its type and each service type.
func NewRootDeviceAdvertisements(uuid string, deviceType Target, serviceTypes ...Target) []Advertisement {
	advertisements := []Advertisement{
		NewAdvertisement(uuid, NewRootDeviceTarget()),
		NewAdvertisement(uuid, NewUuidTarget(uuid)),
		NewAdvertisement(uuid, deviceType),
	}

	for _, serviceType := range serviceTypes {
		advertisements = append(advertisements, NewAdvertisement(uuid, serviceType))
	}

	return advertisements
}

type DeviceConfig struct {
	// Group is DefaultGroup when zero.
	Group Group

	Advertisements []Advertisement
	Location       string
	Server         ProductTokens
	MaxAge         time.Duration
	BootId         Optional[int]
	ConfigId       Optional[int]

	// Extensions go out in every announcement and answer.
	Extensions http.Header
}

func (c DeviceConfig) Validate() error {
	violations := make([]error, 0)

	if len(c.Advertisements) == 0 {
		violations = append(violations, errors.New("at least one advertisement is required"))
	}

	if location, err := url.Parse(c.Location); err != nil || !location.IsAbs() || location.Host == "" {
		violations = append(violations, fmt.Errorf("location %q is not an absolute URL", c.Location))
	}

	if c.Server.IsZero() {
		violations = append(violations, errors.New("server is required"))
	}

	if c.MaxAge < time.Second {
		violations = append(violations, fmt.Errorf("max age %s is shorter than a second", c.MaxAge))
	}

	if c.Server.Upnp().AtLeast(Version11) && (!c.BootId.IsPresent() || !c.ConfigId.IsPresent()) {
		violations = append(violations, fmt.Errorf("UDA %s requires a boot id and a config id", c.Server.Upnp()))
	}

	return errors.Join(violations...)
}

type device struct {
	config   DeviceConfig
	listener endpoint
	senders  []endpoint
	cancel   context.CancelFunc
	done     chan struct{}
	close    sync.Once
}

// NewDevice starts answering searches and announcing config.Advertisements right away.
func NewDevice(config DeviceConfig) (Device, error) {
	config.Group = config.Group.orDefault()

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid device configuration: %w", err)
	}

	listener, err := openListener(config.Group)
	if err != nil {
		return nil, err
	}

	senders, err := openSenders(config.Group)
	if err != nil {
		closeAll(listener)

		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	d := &device{
		config:   config,
		listener: listener,
		senders:  senders,
		cancel:   cancel,
		done:     make(chan struct{}),
	}

	go d.run(ctx, listen(ctx, listener))

	return d, nil
}

func (d *device) Close() error {
	d.close.Do(func() {
		d.cancel()
		<-d.done

		d.announce(context.Background(), d.byeByes())
		closeAll(d.senders...)
	})

	return nil
}

func (d *device) run(ctx context.Context, messages <-chan Received[Message]) {
	defer close(d.done)

	if !sleep(ctx, rand.N(initialDelayCeiling)) {
		return
	}

	d.announce(ctx, d.alives())

	refresh := time.NewTimer(d.refreshInterval())
	defer refresh.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case received, ok := <-messages:
			if !ok {
				return
			}

			if search, isSearch := received.Message.(Search); isSearch {
				d.answer(ctx, search, received.From)
			}
		case <-refresh.C:
			d.announce(ctx, d.alives())
			refresh.Reset(d.refreshInterval())
		}
	}
}

func (d *device) refreshInterval() time.Duration {
	quarter := d.config.MaxAge / 4

	return quarter + rand.N(quarter)
}

func (d *device) announce(ctx context.Context, messages []Message) {
	packets := marshalAll(messages)

	for attempt := range repetitions {
		if attempt > 0 && !sleep(ctx, repetitionInterval) {
			return
		}

		for _, packet := range packets {
			_ = send(ctx, d.senders, d.config.Group.Address(), packet)
		}
	}
}

func (d *device) answer(ctx context.Context, search Search, from netip.AddrPort) {
	if search.IsMulticast() && search.MaxWait < MaxWaitFloor {
		slog.DebugContext(ctx, logIgnored, logKeyFrom, from, logKeyType, search.Type(), logKeyReason, "multicast search without MX")

		return
	}

	packets := marshalAll(d.responses(search))
	if len(packets) == 0 {
		return
	}

	delay := time.Duration(0)
	if search.IsMulticast() {
		delay = rand.N(responseWindow(search.MaxWait))
	}

	time.AfterFunc(delay, func() {
		if ctx.Err() != nil {
			return
		}

		for _, packet := range packets {
			_, _ = d.listener.conn.WriteToUDPAddrPort(packet, from)
		}
	})
}

func responseWindow(maxWait time.Duration) time.Duration {
	return min(maxWait, MaxWaitCeiling) - responseMargin
}

func (d *device) responses(search Search) []Message {
	responses := make([]Message, 0)

	for _, advertisement := range d.config.Advertisements {
		if !advertisement.Target.Answers(search.Target) {
			continue
		}

		responses = append(responses, SearchResponse{
			Target:      advertisement.Target.responseTarget(search.Target),
			ServiceName: advertisement.ServiceName,
			Location:    d.config.Location,
			Server:      d.config.Server,
			MaxAge:      d.config.MaxAge,
			Date:        time.Now(),
			BootId:      d.config.BootId,
			ConfigId:    d.config.ConfigId,

			Extensions: d.config.Extensions,
		})
	}

	return responses
}

func (d *device) alives() []Message {
	alives := make([]Message, 0, len(d.config.Advertisements))

	for _, advertisement := range d.config.Advertisements {
		alives = append(alives, Alive{
			Host:        d.config.Group.String(),
			Target:      advertisement.Target,
			ServiceName: advertisement.ServiceName,
			Location:    d.config.Location,
			Server:      d.config.Server,
			MaxAge:      d.config.MaxAge,
			BootId:      d.config.BootId,
			ConfigId:    d.config.ConfigId,

			Extensions: d.config.Extensions,
		})
	}

	return alives
}

func (d *device) byeByes() []Message {
	byeByes := make([]Message, 0, len(d.config.Advertisements))

	for _, advertisement := range d.config.Advertisements {
		byeByes = append(byeByes, ByeBye{
			Host:        d.config.Group.String(),
			Target:      advertisement.Target,
			ServiceName: advertisement.ServiceName,
			BootId:      d.config.BootId,
			ConfigId:    d.config.ConfigId,
		})
	}

	return byeByes
}

func marshalAll(messages []Message) [][]byte {
	packets := make([][]byte, 0, len(messages))

	for _, message := range messages {
		packet, err := message.MarshalBinary()
		if err != nil {
			slog.Debug(logNotSent, logKeyType, message.Type(), logKeyReason, err)

			continue
		}

		packets = append(packets, packet)
	}

	return packets
}
