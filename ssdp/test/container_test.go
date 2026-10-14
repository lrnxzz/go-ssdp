package test

import (
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
)

func Test_InitializeGerbera(t *testing.T) {
	ctx := t.Context()

	gerbera, address, err := InitializeGerbera(t)
	if err != nil {
		t.Fatalf("Failed to initialize Gerbera container: %v", err)
	}
	defer func() {
		if err := gerbera.Terminate(ctx); err != nil {
			t.Fatalf("Failed to terminate Gerbera container: %v", err)
		}
	}()

	if !address.Is4() {
		t.Errorf("Expected Gerbera on an IPv4 address, got %s", address)
	}
}

func Test_InitializeMiniDlna(t *testing.T) {
	ctx := t.Context()

	miniDlna, address, err := InitializeMiniDlna(t)
	if err != nil {
		t.Fatalf("Failed to initialize MiniDLNA container: %v", err)
	}
	defer func() {
		if err := miniDlna.Terminate(ctx); err != nil {
			t.Fatalf("Failed to terminate MiniDLNA container: %v", err)
		}
	}()

	if !address.Is4() {
		t.Errorf("Expected MiniDLNA on an IPv4 address, got %s", address)
	}
}

func Test_InitializeUpnpClientListener(t *testing.T) {
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

	if _, err := UpnpClientMessages(t, client); err != nil {
		t.Fatalf("Failed to read upnp-client output: %v", err)
	}
}

func Test_RunUpnpClientSearch(t *testing.T) {
	if _, err := RunUpnpClientSearch(t, "ssdp:all", time.Second); err != nil {
		t.Fatalf("Failed to run upnp-client search: %v", err)
	}
}

func Test_DockerGatewayAddress(t *testing.T) {
	gateway, err := DockerGatewayAddress(t)
	if err != nil {
		t.Fatalf("Failed to find the Docker gateway: %v", err)
	}

	if !gateway.Is4() {
		t.Errorf("Expected an IPv4 gateway, got %s", gateway)
	}
}
