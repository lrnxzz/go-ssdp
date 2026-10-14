package ssdp

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"sync"
	"time"
)

// Device announces itself and answers searches until it closes.
type Device interface {
	// Close sends ssdp:byebye for every advertisement and stops answering.
	Close() error
}

type DeviceConfig struct {
	// Group is DefaultGroup when zero.
	Group Group

	Uuid string

	// Targets are announced besides the uuid: upnp:rootdevice, the device type and each service type.
	Targets []string

	Location string

	// Server is built from the platform when empty.
	Server string

	// MaxAge is 1800 seconds when zero.
	MaxAge   time.Duration
	ConfigId int

	// Extensions go out in every announcement and answer.
	Extensions http.Header

	// Logger is slog.Default() when nil.
	Logger *slog.Logger
}

func (c DeviceConfig) Validate() error {
	violations := make([]error, 0)

	if c.Uuid == "" {
		violations = append(violations, errors.New("uuid is required"))
	}

	for _, value := range c.Targets {
		target, err := ParseTarget(value)
		if err != nil {
			violations = append(violations, err)
		} else if target.Type() == TargetAll {
			violations = append(violations, fmt.Errorf("%s is searched for, never announced", value))
		}
	}

	if location, err := url.Parse(c.Location); err != nil || !location.IsAbs() || location.Host == "" {
		violations = append(violations, fmt.Errorf("location %q is not an absolute URL", c.Location))
	}

	if c.Server != "" {
		if _, err := ParseProductTokens(c.Server); err != nil {
			violations = append(violations, fmt.Errorf("invalid server: %w", err))
		}
	}

	if c.MaxAge != 0 && c.MaxAge < time.Second {
		violations = append(violations, fmt.Errorf("max age %s is shorter than a second", c.MaxAge))
	}

	if c.ConfigId < 0 || c.ConfigId > configIdCeiling {
		violations = append(violations, fmt.Errorf("config id %d is outside 0-%d", c.ConfigId, configIdCeiling))
	}

	return errors.Join(violations...)
}

// advertisement is one NT and USN pair a device announces and answers searches for.
type advertisement struct {
	target      Target
	serviceName ServiceName
}

type device struct {
	transport      transport
	advertisements []advertisement
	location       string
	server         ProductTokens
	maxAge         time.Duration
	bootId         Optional[int]
	configId       Optional[int]
	extensions     http.Header

	listener endpoint
	senders  []endpoint
	cancel   context.CancelFunc
	done     chan struct{}
	close    sync.Once
}

// NewDevice starts answering searches and announcing itself right away.
func NewDevice(config DeviceConfig) (Device, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid device configuration: %w", err)
	}

	advertisements := []advertisement{
		{
			target:      newUuidTarget(config.Uuid),
			serviceName: newServiceName(config.Uuid, Target{}),
		},
	}

	for _, value := range config.Targets {
		target, _ := ParseTarget(value)

		if slices.ContainsFunc(advertisements, func(known advertisement) bool {
			return known.target == target
		}) {
			continue
		}

		advertisements = append(advertisements, advertisement{
			target:      target,
			serviceName: newServiceName(config.Uuid, target),
		})
	}

	server := defaultProductTokens()
	if config.Server != "" {
		server = readProductTokens(config.Server)
	}

	transport := newTransport(config.Group, config.Logger)

	listener, err := transport.openListener()
	if err != nil {
		return nil, err
	}

	senders, err := transport.openSenders()
	if err != nil {
		closeAll(listener)

		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	d := &device{
		transport:      transport,
		advertisements: advertisements,
		location:       config.Location,
		server:         server,
		maxAge:         cmp.Or(config.MaxAge, maxAgeFloor),

		// §1.2.2 suggests the seconds since 1970 of the first announcement.
		bootId:     NewOptional(int(time.Now().Unix() & counterCeiling)),
		configId:   NewOptional(config.ConfigId),
		extensions: config.Extensions,

		listener: listener,
		senders:  senders,
		cancel:   cancel,
		done:     make(chan struct{}),
	}

	go d.run(ctx, transport.listen(ctx, listener))

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

func (d *device) run(ctx context.Context, messages <-chan received) {
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
		case datagram, ok := <-messages:
			if !ok {
				return
			}

			if request, isRequest := datagram.message.(SearchRequest); isRequest {
				d.answer(ctx, request, datagram.from)
			}
		case <-refresh.C:
			d.announce(ctx, d.alives())
			refresh.Reset(d.refreshInterval())
		}
	}
}

func (d *device) refreshInterval() time.Duration {
	quarter := d.maxAge / 4

	return quarter + rand.N(quarter)
}

func (d *device) announce(ctx context.Context, messages []Message) {
	packets := d.marshal(messages)

	for attempt := range repetitions {
		if attempt > 0 && !sleep(ctx, repetitionInterval) {
			return
		}

		for _, packet := range packets {
			_ = d.transport.send(ctx, d.senders, d.transport.group.Address(), packet)
		}
	}
}

func (d *device) answer(ctx context.Context, request SearchRequest, from netip.AddrPort) {
	if request.IsMulticast() && request.MaxWait < maxWaitFloor {
		d.transport.logger.DebugContext(ctx, "ssdp: message ignored", "from", from, "type", request.Type(), "reason", "multicast search without MX")

		return
	}

	packets := d.marshal(d.responses(request))
	if len(packets) == 0 {
		return
	}

	delay := time.Duration(0)
	if request.IsMulticast() {
		delay = rand.N(responseWindow(request.MaxWait))
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
	return min(maxWait, maxWaitCeiling) - responseMargin
}

func (d *device) responses(request SearchRequest) []Message {
	responses := make([]Message, 0)

	for _, advertised := range d.advertisements {
		if !advertised.target.Answers(request.Target) {
			continue
		}

		responses = append(responses, SearchResponse{
			Target:      advertised.target.responseTarget(request.Target),
			ServiceName: advertised.serviceName,
			Location:    d.location,
			Server:      d.server,
			MaxAge:      d.maxAge,
			Date:        time.Now(),
			BootId:      d.bootId,
			ConfigId:    d.configId,

			Extensions: d.extensions,
		})
	}

	return responses
}

func (d *device) alives() []Message {
	alives := make([]Message, 0, len(d.advertisements))

	for _, advertised := range d.advertisements {
		alives = append(alives, Alive{
			Host:        d.transport.group.String(),
			Target:      advertised.target,
			ServiceName: advertised.serviceName,
			Location:    d.location,
			Server:      d.server,
			MaxAge:      d.maxAge,
			BootId:      d.bootId,
			ConfigId:    d.configId,

			Extensions: d.extensions,
		})
	}

	return alives
}

func (d *device) byeByes() []Message {
	byeByes := make([]Message, 0, len(d.advertisements))

	for _, advertised := range d.advertisements {
		byeByes = append(byeByes, ByeBye{
			Host:        d.transport.group.String(),
			Target:      advertised.target,
			ServiceName: advertised.serviceName,
			BootId:      d.bootId,
			ConfigId:    d.configId,
		})
	}

	return byeByes
}

func (d *device) marshal(messages []Message) [][]byte {
	packets := make([][]byte, 0, len(messages))

	for _, message := range messages {
		packet, err := message.MarshalBinary()
		if err != nil {
			d.transport.logger.Debug("ssdp: datagram not sent", "type", message.Type(), "reason", err)

			continue
		}

		packets = append(packets, packet)
	}

	return packets
}
