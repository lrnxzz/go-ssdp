package ssdp

import (
	"errors"
	"fmt"
	"net"
	"testing"
	"time"
)

const (
	exampleUuid     = "2fac1234-31f8-11b4-a222-08002b34c003"
	exampleServer   = "unix/5.1 UPnP/1.1 MyProduct/1.0"
	exampleDevice   = "urn:schemas-upnp-org:device:MediaRenderer:1"
	exampleLocation = "http://192.0.2.20:49152/description.xml"
	exampleVendor   = "vendor:service"
)

func exampleSearch(t *testing.T) SearchRequest {
	t.Helper()

	return SearchRequest{
		Host:      MulticastAddress,
		Target:    mustTarget(t, exampleDevice),
		MaxWait:   2 * time.Second,
		UserAgent: mustProductTokens(t, exampleServer),
	}
}

func exampleResponse(t *testing.T) SearchResponse {
	t.Helper()

	return SearchResponse{
		Target:      mustTarget(t, exampleDevice),
		ServiceName: newServiceName(exampleUuid, mustTarget(t, exampleDevice)),
		Location:    exampleLocation,
		Server:      mustProductTokens(t, exampleServer),
		MaxAge:      maxAgeFloor,
		Date:        time.Date(2026, time.October, 13, 12, 0, 0, 0, time.UTC),
		BootId:      NewOptional(1),
		ConfigId:    NewOptional(7),
		SearchPort:  NewOptional(49153),
	}
}

func exampleAlive(t *testing.T) Alive {
	t.Helper()

	return Alive{
		Host:        MulticastAddress,
		Target:      mustTarget(t, exampleDevice),
		ServiceName: newServiceName(exampleUuid, mustTarget(t, exampleDevice)),
		Location:    exampleLocation,
		Server:      mustProductTokens(t, exampleServer),
		MaxAge:      maxAgeFloor,
		BootId:      NewOptional(1),
		ConfigId:    NewOptional(7),
		SearchPort:  NewOptional(49153),
	}
}

func exampleByeBye(t *testing.T) ByeBye {
	t.Helper()

	return ByeBye{
		Host:        MulticastAddress,
		Target:      mustTarget(t, exampleDevice),
		ServiceName: newServiceName(exampleUuid, mustTarget(t, exampleDevice)),
		BootId:      NewOptional(1),
		ConfigId:    NewOptional(7),
	}
}

func exampleUpdate(t *testing.T) Update {
	t.Helper()

	return Update{
		Host:        MulticastAddress,
		Target:      mustTarget(t, exampleDevice),
		ServiceName: newServiceName(exampleUuid, mustTarget(t, exampleDevice)),
		Location:    exampleLocation,
		BootId:      NewOptional(1),
		ConfigId:    NewOptional(7),
		NextBootId:  NewOptional(2),
		SearchPort:  NewOptional(49153),
	}
}

func examples(t *testing.T) []Message {
	t.Helper()

	return []Message{
		exampleSearch(t),
		exampleResponse(t),
		exampleAlive(t),
		exampleByeBye(t),
		exampleUpdate(t),
	}
}

func marshal(t testing.TB, message Message) []byte {
	t.Helper()

	data, err := message.MarshalBinary()
	if err != nil {
		t.Fatalf("Failed to marshal %s: %v", message.Type(), err)
	}

	return data
}

func mustDecode[T Message](t *testing.T, data []byte) T {
	t.Helper()

	message, err := Decode(data)
	if err != nil {
		t.Fatalf("Failed to decode %q: %v", data, err)
	}

	typed, ok := message.(T)
	if !ok {
		t.Fatalf("Expected a %T, got %T", typed, message)
	}

	return typed
}

func mustTarget(t *testing.T, value string) Target {
	t.Helper()

	target, err := ParseTarget(value)
	if err != nil {
		t.Fatalf("Failed to parse target %q: %v", value, err)
	}

	return target
}

func mustProductTokens(t *testing.T, value string) ProductTokens {
	t.Helper()

	tokens, err := ParseProductTokens(value)
	if err != nil {
		t.Fatalf("Failed to parse product tokens %q: %v", value, err)
	}

	return tokens
}

func hostOnlyGroup(t *testing.T) Group {
	t.Helper()

	if len(multicastInterfaces()) == 0 {
		t.Skip("Skipping test that needs a multicast interface to loop multicast through")
	}

	conn, err := net.ListenUDP(udpNetwork, &net.UDPAddr{
		IP: net.IPv4zero,
	})
	if err != nil {
		t.Fatalf("Failed to reserve a port: %v", err)
	}
	defer conn.Close()

	group, err := NewGroup(fmt.Sprintf("239.255.255.250:%d", conn.LocalAddr().(*net.UDPAddr).Port), HostOnlyTtl)
	if err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}

	return group
}

func expectConformant(t *testing.T, message Message) {
	t.Helper()

	if err := message.Check(); err != nil {
		t.Errorf("Expected %s to conform, got %v", message.Type(), err)
	}
}

func expectViolations(t *testing.T, message Message, count int) {
	t.Helper()

	err := message.Check()
	if !errors.Is(err, ErrNonConformant) {
		t.Fatalf("Expected %s to break the specification, got %v", message.Type(), err)
	}

	violations, ok := errors.AsType[Violations](err)
	if !ok {
		t.Fatalf("Expected Violations, got %T", err)
	}

	if len(violations) != count {
		t.Errorf("Expected %d violations, got %d: %v", count, len(violations), err)
	}
}
