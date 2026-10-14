package test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"

	"github.com/lrnxzz/go-ssdp/ssdp"
)

const (
	basicDevice  = "urn:schemas-upnp-org:device:Basic:1"
	deviceUuid   = "6f9e1c2a-3b4d-4e5f-8a9b-0c1d2e3f4a5b"
	clientSearch = 4 * time.Second

	advertisementWait = 5 * time.Second
	advertisementPoll = 200 * time.Millisecond
)

func Test_Integration_Device_IsFoundByAnotherControlPoint(t *testing.T) {
	location := startDevice(t)

	answers, err := RunUpnpClientSearch(t, basicDevice, clientSearch)
	if err != nil {
		t.Fatalf("Failed to run upnp-client search: %v", err)
	}

	answer, found := messageFor(answers, "uuid:"+deviceUuid+"::"+basicDevice)
	if !found {
		t.Fatalf("Expected upnp-client to find our device, got %v", answers)
	}

	expected := map[string]string{
		"ST":                basicDevice,
		"LOCATION":          location,
		"CONFIGID.UPNP.ORG": "1",
		"EXT":               "",
		"_udn":              "uuid:" + deviceUuid,
	}

	for field, value := range expected {
		if got, ok := answer[field]; !ok || got != value {
			t.Errorf("Expected %s = %q, got %q", field, value, got)
		}
	}

	if answer["BOOTID.UPNP.ORG"] == "" {
		t.Errorf("Expected a BOOTID.UPNP.ORG, got %v", answer)
	}
}

func Test_Integration_Device_AnnouncesItselfAndSaysByeBye(t *testing.T) {
	ctx := t.Context()

	client, err := InitializeUpnpClientListener(t)
	if err != nil {
		t.Fatalf("Failed to initialize upnp-client container: %v", err)
	}
	defer func() {
		if err := client.Terminate(ctx, testcontainers.StopTimeout(UpnpClientStopTimeout)); err != nil {
			t.Fatalf("Failed to terminate upnp-client container: %v", err)
		}
	}()

	device, _ := newDevice(t)

	awaitSubtype(t, client, "ssdp:alive")

	if err := device.Close(); err != nil {
		t.Fatalf("Failed to close device: %v", err)
	}

	messages := awaitSubtype(t, client, "ssdp:byebye")

	announced := make([]string, 0)

	for _, message := range messages {
		if message["NTS"] == "ssdp:alive" {
			announced = append(announced, message["NT"])
		}
	}

	targets := []string{
		"upnp:rootdevice",
		"uuid:" + deviceUuid,
		basicDevice,
	}

	for _, target := range targets {
		if !slices.Contains(announced, target) {
			t.Errorf("Expected an ssdp:alive for %s, got %v", target, announced)
		}
	}
}

func startDevice(t *testing.T) string {
	t.Helper()

	device, location := newDevice(t)

	t.Cleanup(func() {
		if err := device.Close(); err != nil {
			t.Errorf("Failed to close device: %v", err)
		}
	})

	return location
}

func newDevice(t *testing.T) (ssdp.Device, string) {
	t.Helper()

	gateway, err := DockerGatewayAddress(t)
	if err != nil {
		t.Fatalf("Failed to find the Docker gateway: %v", err)
	}

	location := fmt.Sprintf("http://%s:49152/description.xml", gateway)

	device, err := ssdp.NewDevice(ssdp.DeviceConfig{
		Uuid: deviceUuid,
		Targets: []string{
			"upnp:rootdevice",
			basicDevice,
		},
		Location: location,
		ConfigId: 1,
	})
	if err != nil {
		t.Fatalf("Failed to start device: %v", err)
	}

	return device, location
}

func awaitSubtype(t *testing.T, client *testcontainers.DockerContainer, subtype string) []map[string]string {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), advertisementWait)
	defer cancel()

	for {
		messages, err := UpnpClientMessages(t, client)
		if err != nil {
			t.Fatalf("Failed to read upnp-client output: %v", err)
		}

		if slices.ContainsFunc(messages, isOurs(subtype)) {
			return messages
		}

		select {
		case <-ctx.Done():
			t.Fatalf("Expected upnp-client to print an %s for our device, got %v", subtype, messages)
		case <-time.After(advertisementPoll):
		}
	}
}

func isOurs(subtype string) func(map[string]string) bool {
	return func(message map[string]string) bool {
		return message["NTS"] == subtype && message["_udn"] == "uuid:"+deviceUuid
	}
}

func messageFor(messages []map[string]string, serviceName string) (map[string]string, bool) {
	for _, message := range messages {
		if message["USN"] == serviceName {
			return message, true
		}
	}

	return nil, false
}
