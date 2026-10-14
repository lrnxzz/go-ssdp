package ssdp

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"sync"
	"time"

	"golang.org/x/net/ipv4"
)

// transport is the network side of a control point or a device: the group it talks to and the logger
// its sockets report to.
type transport struct {
	group  Group
	logger *slog.Logger
}

func newTransport(group Group, logger *slog.Logger) transport {
	return transport{
		group:  group.orDefault(),
		logger: cmp.Or(logger, slog.Default()),
	}
}

type endpoint struct {
	conn          *net.UDPConn
	interfaceName string
}

func newEndpoint(conn *net.UDPConn, interfaceName string) endpoint {
	return endpoint{
		conn:          conn,
		interfaceName: interfaceName,
	}
}

// received is a decoded message, the address it came from and the local interface it arrived on.
type received struct {
	message       Message
	from          netip.AddrPort
	interfaceName string
}

func (t transport) openSenders() ([]endpoint, error) {
	interfaces := multicastInterfaces()
	if len(interfaces) == 0 {
		return t.openDefaultSender()
	}

	endpoints := make([]endpoint, 0, len(interfaces))

	for _, iface := range interfaces {
		sender, err := t.openSender(iface)
		if err != nil {
			t.logger.Debug("ssdp: interface skipped", "interface", iface.Name, "reason", err)

			continue
		}

		endpoints = append(endpoints, sender)
	}

	if len(endpoints) == 0 {
		return nil, fmt.Errorf("failed to open a socket on any of %d interfaces", len(interfaces))
	}

	return endpoints, nil
}

func (t transport) openSender(iface net.Interface) (endpoint, error) {
	address, _ := interfaceAddress(iface)

	conn, err := net.ListenUDP(udpNetwork, net.UDPAddrFromAddrPort(netip.AddrPortFrom(address, 0)))
	if err != nil {
		return endpoint{}, err
	}

	if err := t.configureMulticast(conn, &iface); err != nil {
		_ = conn.Close()

		return endpoint{}, err
	}

	return newEndpoint(conn, iface.Name), nil
}

func (t transport) openDefaultSender() ([]endpoint, error) {
	conn, err := net.ListenUDP(udpNetwork, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open a socket: %w", err)
	}

	if err := t.configureMulticast(conn, nil); err != nil {
		_ = conn.Close()

		return nil, err
	}

	return []endpoint{
		newEndpoint(conn, defaultInterface),
	}, nil
}

func (t transport) configureMulticast(conn *net.UDPConn, iface *net.Interface) error {
	packetConn := ipv4.NewPacketConn(conn)

	if iface != nil {
		if err := packetConn.SetMulticastInterface(iface); err != nil {
			return fmt.Errorf("failed to send multicast through %s: %w", iface.Name, err)
		}
	}

	if err := packetConn.SetMulticastTTL(t.group.ttl); err != nil {
		return fmt.Errorf("failed to set multicast ttl: %w", err)
	}

	if err := packetConn.SetMulticastLoopback(true); err != nil {
		return fmt.Errorf("failed to enable multicast loopback: %w", err)
	}

	return nil
}

func (t transport) openListener() (endpoint, error) {
	config := net.ListenConfig{
		Control: reuseAddress,
	}

	address := net.JoinHostPort("0.0.0.0", strconv.Itoa(int(t.group.address.Port())))

	packetConn, err := config.ListenPacket(context.Background(), udpNetwork, address)
	if err != nil {
		return endpoint{}, fmt.Errorf("failed to listen on %s: %w", address, err)
	}

	conn, ok := packetConn.(*net.UDPConn)
	if !ok {
		_ = packetConn.Close()

		return endpoint{}, fmt.Errorf("listening on %s gave no UDP socket", address)
	}

	if err := t.joinGroup(conn); err != nil {
		_ = conn.Close()

		return endpoint{}, err
	}

	return newEndpoint(conn, "group"), nil
}

func (t transport) joinGroup(conn *net.UDPConn) error {
	packetConn := ipv4.NewPacketConn(conn)
	groupAddress := net.UDPAddrFromAddrPort(t.group.address)
	joined := 0

	for _, iface := range multicastInterfaces() {
		if err := packetConn.JoinGroup(&iface, groupAddress); err != nil {
			t.logger.Debug("ssdp: interface skipped", "interface", iface.Name, "reason", err)

			continue
		}

		joined++
	}

	if joined > 0 {
		return nil
	}

	if err := packetConn.JoinGroup(nil, groupAddress); err != nil {
		return fmt.Errorf("failed to join %s on any interface: %w", t.group, err)
	}

	return nil
}

func (t transport) listen(ctx context.Context, endpoints ...endpoint) <-chan received {
	messages := make(chan received)

	var loops sync.WaitGroup

	for _, current := range endpoints {
		loops.Go(func() {
			t.read(ctx, current, messages)
		})
	}

	stop := context.AfterFunc(ctx, func() {
		closeAll(endpoints...)
	})

	go func() {
		loops.Wait()
		stop()
		closeAll(endpoints...)
		close(messages)
	}()

	return messages
}

func (t transport) read(ctx context.Context, current endpoint, messages chan<- received) {
	buffer := make([]byte, maxDatagramSize)

	for {
		size, from, err := current.conn.ReadFromUDPAddrPort(buffer)
		if err != nil {
			return
		}

		from = netip.AddrPortFrom(from.Addr().Unmap(), from.Port())

		message, err := Decode(buffer[:size])
		if err != nil {
			t.logger.DebugContext(ctx, "ssdp: datagram dropped", "from", from, "interface", current.interfaceName, "reason", err)

			continue
		}

		datagram := received{
			message:       message,
			from:          from,
			interfaceName: current.interfaceName,
		}

		select {
		case messages <- datagram:
		case <-ctx.Done():
			return
		}
	}
}

func (t transport) send(ctx context.Context, endpoints []endpoint, to netip.AddrPort, packet []byte) error {
	sent := 0

	for _, current := range endpoints {
		if _, err := current.conn.WriteToUDPAddrPort(packet, to); err != nil {
			t.logger.DebugContext(ctx, "ssdp: datagram not sent", "interface", current.interfaceName, "reason", err)

			continue
		}

		sent++
	}

	if sent == 0 {
		return fmt.Errorf("failed to send to %s from any of %d sockets", to, len(endpoints))
	}

	return nil
}

func (t transport) repeat(ctx context.Context, endpoints []endpoint, to netip.AddrPort, packet []byte) {
	for range repetitions - 1 {
		if !sleep(ctx, repetitionInterval) {
			return
		}

		_ = t.send(ctx, endpoints, to, packet)
	}
}

func multicastInterfaces() []net.Interface {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	return slices.DeleteFunc(interfaces, isUnusable)
}

func isUnusable(iface net.Interface) bool {
	if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 || iface.Flags&net.FlagLoopback != 0 {
		return true
	}

	_, ok := interfaceAddress(iface)

	return !ok
}

func interfaceAddress(iface net.Interface) (netip.Addr, bool) {
	addresses, err := iface.Addrs()
	if err != nil {
		return netip.Addr{}, false
	}

	for _, address := range addresses {
		prefix, err := netip.ParsePrefix(address.String())
		if err == nil && prefix.Addr().Is4() {
			return prefix.Addr(), true
		}
	}

	return netip.Addr{}, false
}

func closeAll(endpoints ...endpoint) {
	for _, current := range endpoints {
		_ = current.conn.Close()
	}
}

func sleep(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
