package ssdp

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"
)

const receiveTimeout = 2 * time.Second

func Test_Listen_DeliversWhatIsSentToTheGroup(t *testing.T) {
	current := newTransport(hostOnlyGroup(t), nil)
	listener, senders := openEndpoints(t, current)

	messages := current.listen(t.Context(), listener)

	if err := current.send(t.Context(), senders, current.group.Address(), marshal(t, exampleAlive(t))); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	datagram := receive(t, messages)

	if datagram.message.Type() != MessageAlive || datagram.interfaceName != "group" || !datagram.from.IsValid() {
		t.Errorf("Expected an alive on the group listener from a valid address, got %+v", datagram)
	}
}

func Test_Listen_DropsDatagramsThatAreNotSsdp(t *testing.T) {
	current := newTransport(hostOnlyGroup(t), nil)
	listener, senders := openEndpoints(t, current)

	messages := current.listen(t.Context(), listener)

	if err := current.send(t.Context(), senders, current.group.Address(), []byte("not ssdp at all")); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	if err := current.send(t.Context(), senders, current.group.Address(), marshal(t, exampleByeBye(t))); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	if datagram := receive(t, messages); datagram.message.Type() != MessageByeBye {
		t.Errorf("Expected the byebye after the dropped datagram, got %s", datagram.message.Type())
	}
}

func Test_Listen_StopsWhenTheContextEnds(t *testing.T) {
	current := newTransport(hostOnlyGroup(t), nil)
	listener, _ := openEndpoints(t, current)

	ctx, cancel := context.WithCancel(t.Context())
	messages := current.listen(ctx, listener)

	cancel()

	select {
	case _, open := <-messages:
		if open {
			t.Error("Expected no message after the context ended")
		}
	case <-time.After(receiveTimeout):
		t.Fatal("Expected the stream to close once the context ended")
	}
}

func Test_OpenListener_SharesThePort(t *testing.T) {
	current := newTransport(hostOnlyGroup(t), nil)
	first, senders := openEndpoints(t, current)

	second, err := current.openListener()
	if err != nil {
		t.Fatalf("Expected a second listener on the same port, got %v", err)
	}

	firstMessages := current.listen(t.Context(), first)
	secondMessages := current.listen(t.Context(), second)

	if err := current.send(t.Context(), senders, current.group.Address(), marshal(t, exampleAlive(t))); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	receive(t, firstMessages)
	receive(t, secondMessages)
}

func Test_Send_FailsWhenNoSocketCanSend(t *testing.T) {
	conn, err := net.ListenUDP(udpNetwork, nil)
	if err != nil {
		t.Fatalf("Failed to open a socket: %v", err)
	}

	_ = conn.Close()

	closed := []endpoint{
		newEndpoint(conn, defaultInterface),
	}

	if err := newTransport(Group{}, nil).send(t.Context(), closed, netip.MustParseAddrPort("127.0.0.1:9"), []byte("x")); err == nil {
		t.Error("Expected an error when every socket is closed")
	}
}

func Test_Repeat_SendsTheRemainingRepetitions(t *testing.T) {
	current := newTransport(hostOnlyGroup(t), nil)
	listener, senders := openEndpoints(t, current)

	messages := current.listen(t.Context(), listener)

	current.repeat(t.Context(), senders, current.group.Address(), marshal(t, exampleSearch(t)))

	for range (repetitions - 1) * len(senders) {
		receive(t, messages)
	}
}

func Test_NewTransport_FillsTheDefaults(t *testing.T) {
	current := newTransport(Group{}, nil)

	if current.group != DefaultGroup || current.logger == nil {
		t.Errorf("Expected the default group and a logger, got %+v", current)
	}
}

func Test_MulticastInterfaces_AreUpWithIpv4AndNotLoopback(t *testing.T) {
	for _, iface := range multicastInterfaces() {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 {
			t.Errorf("Expected %s to be up, multicast and not loopback, got %s", iface.Name, iface.Flags)
		}

		if address, ok := interfaceAddress(iface); !ok || !address.Is4() {
			t.Errorf("Expected %s to have an IPv4 address, got %s", iface.Name, address)
		}
	}
}

func Test_OpenSenders_BindsEachSocketToItsInterface(t *testing.T) {
	_, senders := openEndpoints(t, newTransport(hostOnlyGroup(t), nil))

	for _, sender := range senders {
		local := sender.conn.LocalAddr().(*net.UDPAddr)

		if local.Port == 0 || local.IP.IsUnspecified() {
			t.Errorf("Expected %s bound to its own address, got %s", sender.interfaceName, local)
		}
	}
}

func Test_Sleep(t *testing.T) {
	if !sleep(t.Context(), time.Millisecond) {
		t.Error("Expected a short sleep to finish")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	started := time.Now()

	if sleep(ctx, time.Hour) || time.Since(started) > time.Second {
		t.Error("Expected a canceled sleep to return at once")
	}
}

func openEndpoints(t *testing.T, current transport) (endpoint, []endpoint) {
	t.Helper()

	listener, err := current.openListener()
	if err != nil {
		t.Fatalf("Failed to open the listener: %v", err)
	}

	senders, err := current.openSenders()
	if err != nil {
		t.Fatalf("Failed to open the senders: %v", err)
	}

	t.Cleanup(func() {
		closeAll(listener)
		closeAll(senders...)
	})

	return listener, senders
}

func receive(t *testing.T, messages <-chan received) received {
	t.Helper()

	select {
	case datagram, open := <-messages:
		if !open {
			t.Fatal("Expected a message, the stream closed")
		}

		return datagram
	case <-time.After(receiveTimeout):
		t.Fatal("Expected a message within the timeout")
	}

	return received{}
}
