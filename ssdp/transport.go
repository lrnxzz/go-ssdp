package ssdp

import (
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

func openSenders(group Group) ([]endpoint, error) {
	interfaces := multicastInterfaces()
	if len(interfaces) == 0 {
		return openDefaultSender(group)
	}

	endpoints := make([]endpoint, 0, len(interfaces))

	for _, iface := range interfaces {
		sender, err := openSender(iface, group)
		if err != nil {
			slog.Debug(logSkipped, logKeyInterface, iface.Name, logKeyReason, err)

			continue
		}

		endpoints = append(endpoints, sender)
	}

	if len(endpoints) == 0 {
		return nil, fmt.Errorf("failed to open a socket on any of %d interfaces", len(interfaces))
	}

	return endpoints, nil
}

func openSender(iface net.Interface, group Group) (endpoint, error) {
	address, _ := interfaceAddress(iface)

	conn, err := net.ListenUDP(udpNetwork, net.UDPAddrFromAddrPort(netip.AddrPortFrom(address, 0)))
	if err != nil {
		return endpoint{}, err
	}

	if err := configureMulticast(conn, &iface, group); err != nil {
		_ = conn.Close()

		return endpoint{}, err
	}

	return newEndpoint(conn, iface.Name), nil
}

func openDefaultSender(group Group) ([]endpoint, error) {
	conn, err := net.ListenUDP(udpNetwork, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open a socket: %w", err)
	}

	if err := configureMulticast(conn, nil, group); err != nil {
		_ = conn.Close()

		return nil, err
	}

	return []endpoint{
		newEndpoint(conn, defaultInterface),
	}, nil
}

func configureMulticast(conn *net.UDPConn, iface *net.Interface, group Group) error {
	packetConn := ipv4.NewPacketConn(conn)

	if iface != nil {
		if err := packetConn.SetMulticastInterface(iface); err != nil {
			return fmt.Errorf("failed to send multicast through %s: %w", iface.Name, err)
		}
	}

	if err := packetConn.SetMulticastTTL(group.ttl); err != nil {
		return fmt.Errorf("failed to set multicast ttl: %w", err)
	}

	if err := packetConn.SetMulticastLoopback(true); err != nil {
		return fmt.Errorf("failed to enable multicast loopback: %w", err)
	}

	return nil
}

func openListener(group Group) (endpoint, error) {
	config := net.ListenConfig{
		Control: reuseAddress,
	}

	address := net.JoinHostPort(allInterfaces, strconv.Itoa(int(group.address.Port())))

	packetConn, err := config.ListenPacket(context.Background(), udpNetwork, address)
	if err != nil {
		return endpoint{}, fmt.Errorf("failed to listen on %s: %w", address, err)
	}

	conn, ok := packetConn.(*net.UDPConn)
	if !ok {
		_ = packetConn.Close()

		return endpoint{}, fmt.Errorf("listening on %s gave no UDP socket", address)
	}

	if err := joinGroup(conn, group); err != nil {
		_ = conn.Close()

		return endpoint{}, err
	}

	return newEndpoint(conn, listenerName), nil
}

func joinGroup(conn *net.UDPConn, group Group) error {
	packetConn := ipv4.NewPacketConn(conn)
	groupAddress := net.UDPAddrFromAddrPort(group.address)
	joined := 0

	for _, iface := range multicastInterfaces() {
		if err := packetConn.JoinGroup(&iface, groupAddress); err != nil {
			slog.Debug(logSkipped, logKeyInterface, iface.Name, logKeyReason, err)

			continue
		}

		joined++
	}

	if joined > 0 {
		return nil
	}

	if err := packetConn.JoinGroup(nil, groupAddress); err != nil {
		return fmt.Errorf("failed to join %s on any interface: %w", group, err)
	}

	return nil
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

func listen(ctx context.Context, endpoints ...endpoint) <-chan Received[Message] {
	messages := make(chan Received[Message])

	var loops sync.WaitGroup

	for _, current := range endpoints {
		loops.Go(func() {
			current.read(ctx, messages)
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

func (e endpoint) read(ctx context.Context, messages chan<- Received[Message]) {
	buffer := make([]byte, maxDatagramSize)

	for {
		size, from, err := e.conn.ReadFromUDPAddrPort(buffer)
		if err != nil {
			return
		}

		from = netip.AddrPortFrom(from.Addr().Unmap(), from.Port())

		message, err := Decode(buffer[:size])
		if err != nil {

			slog.DebugContext(ctx, logDropped, logKeyFrom, from, logKeyInterface, e.interfaceName, logKeyReason, err)

			continue
		}

		received := Received[Message]{
			Message:   message,
			From:      from,
			Interface: e.interfaceName,
		}

		select {
		case messages <- received:
		case <-ctx.Done():
			return
		}
	}
}

func send(ctx context.Context, endpoints []endpoint, to netip.AddrPort, packet []byte) error {
	sent := 0

	for _, current := range endpoints {
		if _, err := current.conn.WriteToUDPAddrPort(packet, to); err != nil {
			slog.DebugContext(ctx, logNotSent, logKeyInterface, current.interfaceName, logKeyReason, err)

			continue
		}

		sent++
	}

	if sent == 0 {
		return fmt.Errorf("failed to send to %s from any of %d sockets", to, len(endpoints))
	}

	return nil
}

func repeat(ctx context.Context, endpoints []endpoint, to netip.AddrPort, packet []byte) {
	for range repetitions - 1 {
		if !sleep(ctx, repetitionInterval) {
			return
		}

		_ = send(ctx, endpoints, to, packet)
	}
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

// Received is a decoded message, the address it came from and the local interface it arrived on.
type Received[T any] struct {
	Message   T
	From      netip.AddrPort
	Interface string
}
