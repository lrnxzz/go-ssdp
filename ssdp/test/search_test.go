package test

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/netip"
	"slices"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"

	"github.com/lrnxzz/go-ssdp/ssdp"
)

const (
	mediaServer = "urn:schemas-upnp-org:device:MediaServer:1"
	searchWait  = 2 * time.Second

	unicastSearchCeiling = 2 * time.Second
)

type server func(t *testing.T) (*testcontainers.DockerContainer, netip.Addr, error)

type description struct {
	Device descriptionDevice `xml:"device"`
}

type descriptionDevice struct {
	DeviceType string `xml:"deviceType"`
	Udn        string `xml:"UDN"`
}

func servers() map[string]server {
	return map[string]server{
		"Gerbera":  InitializeGerbera,
		"MiniDLNA": InitializeMiniDlna,
	}
}

func Test_Integration_Search_FindsEveryAdvertisement(t *testing.T) {
	for name, initialize := range servers() {
		t.Run(name, func(t *testing.T) {
			address := start(t, initialize)
			responses := searchFrom(t, address, ssdp.NewAllTarget())

			uuid := responses[0].ServiceName.Uuid()
			targets := make([]string, 0, len(responses))

			for _, response := range responses {
				targets = append(targets, response.Target.String())
			}

			expected := []string{
				"upnp:rootdevice",
				"uuid:" + uuid,
				mediaServer,
			}

			for _, target := range expected {
				if !slices.Contains(targets, target) {
					t.Errorf("Expected %s to announce %s, got %v", name, target, targets)
				}
			}
		})
	}
}

func Test_Integration_Search_PairsServiceNameWithTarget(t *testing.T) {
	for name, initialize := range servers() {
		t.Run(name, func(t *testing.T) {
			address := start(t, initialize)

			for _, response := range searchFrom(t, address, ssdp.NewAllTarget()) {
				expected := ssdp.NewServiceName(response.ServiceName.Uuid(), response.Target)

				if response.ServiceName != expected {
					t.Errorf("Expected USN %s for ST %s, got %s", expected, response.Target, response.ServiceName)
				}
			}
		})
	}
}

func Test_Integration_Search_AnswersConformToTheirVersion(t *testing.T) {
	for name, initialize := range servers() {
		t.Run(name, func(t *testing.T) {
			address := start(t, initialize)

			for _, response := range searchFrom(t, address, ssdp.NewAllTarget()) {
				if response.Server.Upnp().IsZero() {
					t.Errorf("Expected %s to declare a UPnP version, got SERVER %q", name, response.Server)
				}

				if err := response.Check(); err != nil {
					t.Errorf("Expected the answer for %s to conform, got %v", response.Target, err)
				}
			}
		})
	}
}

func Test_Integration_Search_LocationDescribesTheSameDevice(t *testing.T) {
	for name, initialize := range servers() {
		t.Run(name, func(t *testing.T) {
			address := start(t, initialize)
			response := searchFrom(t, address, mustTarget(t, mediaServer))[0]

			described := describe(t, response.Location)

			if described.Device.Udn != "uuid:"+response.ServiceName.Uuid() {
				t.Errorf("Expected UDN uuid:%s, got %s", response.ServiceName.Uuid(), described.Device.Udn)
			}

			if described.Device.DeviceType != mediaServer {
				t.Errorf("Expected device type %s, got %s", mediaServer, described.Device.DeviceType)
			}
		})
	}
}

func Test_Integration_Search_EchoesTheSearchedTarget(t *testing.T) {
	for name, initialize := range servers() {
		t.Run(name, func(t *testing.T) {
			address := start(t, initialize)

			for _, response := range searchFrom(t, address, mustTarget(t, mediaServer)) {
				if response.Target.String() != mediaServer {
					t.Errorf("Expected ST %s, got %s", mediaServer, response.Target)
				}
			}
		})
	}
}

func Test_Integration_SearchUnicast_FollowsTheDeclaredVersion(t *testing.T) {
	for name, initialize := range servers() {
		t.Run(name, func(t *testing.T) {
			address := start(t, initialize)
			declared := searchFrom(t, address, ssdp.NewRootDeviceTarget())[0].Server.Upnp()

			started := time.Now()
			answered := false

			for received, err := range newControlPoint(t).SearchUnicast(t.Context(), netip.AddrPortFrom(address, 1900), ssdp.NewRootDeviceTarget()) {
				if err != nil {
					t.Fatalf("Failed to search %s: %v", address, err)
				}

				answered = answered || received.From.Addr() == address
			}

			if declared.AtLeast(ssdp.Version11) && !answered {
				t.Errorf("Expected %s, declaring UDA %s, to answer a unicast search", name, declared)
			}

			if elapsed := time.Since(started); elapsed > unicastSearchCeiling {
				t.Errorf("Expected the unicast search to end within %s, took %s", unicastSearchCeiling, elapsed)
			}
		})
	}
}

func start(t *testing.T, initialize server) netip.Addr {
	t.Helper()

	container, address, err := initialize(t)
	if err != nil {
		t.Fatalf("Failed to initialize container: %v", err)
	}

	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Errorf("Failed to terminate container: %v", err)
		}
	})

	return address
}

func newControlPoint(t *testing.T) ssdp.ControlPoint {
	t.Helper()

	controlPoint, err := ssdp.NewControlPoint(ssdp.ControlPointConfig{})
	if err != nil {
		t.Fatalf("Failed to create control point: %v", err)
	}

	return controlPoint
}

func searchFrom(t *testing.T, address netip.Addr, target ssdp.Target) []ssdp.SearchResponse {
	t.Helper()

	responses := make([]ssdp.SearchResponse, 0)

	for received, err := range newControlPoint(t).Search(t.Context(), target, searchWait) {
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}

		if received.From.Addr() == address {
			responses = append(responses, received.Message)
		}
	}

	if len(responses) == 0 {
		t.Fatalf("Expected %s to answer a search for %s", address, target)
	}

	return responses
}

func describe(t *testing.T, location string) description {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, location, nil)
	if err != nil {
		t.Fatalf("Failed to build a request for %s: %v", location, err)
	}

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("Failed to fetch %s: %v", location, err)
	}
	defer resp.Body.Close()

	var described description
	if err := xml.NewDecoder(resp.Body).Decode(&described); err != nil {
		t.Fatalf("Failed to read the description at %s: %v", location, err)
	}

	return described
}

func mustTarget(t *testing.T, value string) ssdp.Target {
	t.Helper()

	target, err := ssdp.ParseTarget(value)
	if err != nil {
		t.Fatalf("Failed to parse target %q: %v", value, err)
	}

	return target
}
