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
	group := hostOnlyGroup(t)
	listener, senders := openEndpoints(t, group)

	messages := listen(t.Context(), listener)

	if err := send(t.Context(), senders, group.Address(), marshal(t, exampleAlive(t))); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	received := receive(t, messages)

	if received.Message.Type() != MessageAlive || received.Interface != listenerName || !received.From.IsValid() {
		t.Errorf("Expected an alive on the group listener from a valid address, got %+v", received)
	}
}

func Test_Listen_DropsDatagramsThatAreNotSsdp(t *testing.T) {
	group := hostOnlyGroup(t)
	listener, senders := openEndpoints(t, group)

	messages := listen(t.Context(), listener)

	if err := send(t.Context(), senders, group.Address(), []byte("not ssdp at all")); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	if err := send(t.Context(), senders, group.Address(), marshal(t, exampleByeBye(t))); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	if received := receive(t, messages); received.Message.Type() != MessageByeBye {
		t.Errorf("Expected the byebye after the dropped datagram, got %s", received.Message.Type())
	}
}

func Test_Listen_StopsWhenTheContextEnds(t *testing.T) {
	group := hostOnlyGroup(t)
	listener, _ := openEndpoints(t, group)

	ctx, cancel := context.WithCancel(t.Context())
	messages := listen(ctx, listener)

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
	group := hostOnlyGroup(t)
	first, senders := openEndpoints(t, group)

	second, err := openListener(group)
	if err != nil {
		t.Fatalf("Expected a second listener on the same port, got %v", err)
	}

	firstMessages := listen(t.Context(), first)
	secondMessages := listen(t.Context(), second)

	if err := send(t.Context(), senders, group.Address(), marshal(t, exampleAlive(t))); err != nil {
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

	if err := send(t.Context(), closed, netip.MustParseAddrPort("127.0.0.1:9"), []byte("x")); err == nil {
		t.Error("Expected an error when every socket is closed")
	}
}

func Test_Repeat_SendsTheRemainingRepetitions(t *testing.T) {
	group := hostOnlyGroup(t)
	listener, senders := openEndpoints(t, group)

	messages := listen(t.Context(), listener)

	repeat(t.Context(), senders, group.Address(), marshal(t, exampleSearch(t)))

	for range (repetitions - 1) * len(senders) {
		receive(t, messages)
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
	group := hostOnlyGroup(t)
	_, senders := openEndpoints(t, group)

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

func openEndpoints(t *testing.T, group Group) (endpoint, []endpoint) {
	t.Helper()

	listener, err := openListener(group)
	if err != nil {
		t.Fatalf("Failed to open the listener: %v", err)
	}

	senders, err := openSenders(group)
	if err != nil {
		t.Fatalf("Failed to open the senders: %v", err)
	}

	t.Cleanup(func() {
		closeAll(listener)
		closeAll(senders...)
	})

	return listener, senders
}

func receive(t *testing.T, messages <-chan Received[Message]) Received[Message] {
	t.Helper()

	select {
	case received, open := <-messages:
		if !open {
			t.Fatal("Expected a message, the stream closed")
		}

		return received
	case <-time.After(receiveTimeout):
		t.Fatal("Expected a message within the timeout")
	}

	return Received[Message]{}
}
