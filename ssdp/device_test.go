package ssdp

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func Test_ResponseWindow(t *testing.T) {
	tests := map[time.Duration]time.Duration{
		time.Second:      500 * time.Millisecond,
		2 * time.Second:  1500 * time.Millisecond,
		5 * time.Second:  4500 * time.Millisecond,
		10 * time.Second: 4500 * time.Millisecond,
	}

	for maxWait, expected := range tests {
		if got := responseWindow(maxWait); got != expected {
			t.Errorf("Expected a %s window for MX %s, got %s", expected, maxWait, got)
		}
	}
}

func Test_NewRootDeviceAdvertisements(t *testing.T) {
	service := mustTarget(t, "urn:schemas-upnp-org:service:AVTransport:1")
	advertisements := NewRootDeviceAdvertisements(exampleUuid, mustTarget(t, exampleDevice), service)

	expected := []string{
		"upnp:rootdevice uuid:" + exampleUuid + "::upnp:rootdevice",
		"uuid:" + exampleUuid + " uuid:" + exampleUuid,
		exampleDevice + " uuid:" + exampleUuid + "::" + exampleDevice,
		service.String() + " uuid:" + exampleUuid + "::" + service.String(),
	}

	if len(advertisements) != len(expected) {
		t.Fatalf("Expected %d advertisements, got %d", len(expected), len(advertisements))
	}

	for index, advertisement := range advertisements {
		if got := advertisement.Target.String() + " " + advertisement.ServiceName.String(); got != expected[index] {
			t.Errorf("Expected advertisement %d to be %q, got %q", index, expected[index], got)
		}
	}
}

func Test_DeviceConfig_Validate(t *testing.T) {
	valid := DeviceConfig{
		Advertisements: NewRootDeviceAdvertisements(exampleUuid, mustTarget(t, exampleDevice)),
		Location:       exampleLocation,
		Server:         mustProductTokens(t, exampleServer),
		MaxAge:         MaxAgeFloor,
		BootId:         NewOptional(1),
		ConfigId:       NewOptional(1),
	}

	if err := valid.Validate(); err != nil {
		t.Errorf("Expected a valid configuration, got %v", err)
	}

	noAdvertisements := valid
	noAdvertisements.Advertisements = nil

	relativeLocation := valid
	relativeLocation.Location = "/description.xml"

	noServer := valid
	noServer.Server = ProductTokens{}

	shortMaxAge := valid
	shortMaxAge.MaxAge = 500 * time.Millisecond

	noCounters := valid
	noCounters.BootId = Optional[int]{}

	tests := map[string]DeviceConfig{
		"no advertisements": noAdvertisements,
		"relative location": relativeLocation,
		"no server":         noServer,
		"short max age":     shortMaxAge,
		"no counters":       noCounters,
	}

	for name, config := range tests {
		if err := config.Validate(); err == nil {
			t.Errorf("Expected %s to be refused", name)
		}
	}
}

func Test_DeviceConfig_Validate_ReportsEveryProblem(t *testing.T) {
	err := (DeviceConfig{}).Validate()
	if err == nil {
		t.Fatal("Expected an empty configuration to be refused")
	}

	problems := []string{
		"advertisement",
		"location",
		"server",
		"max age",
	}

	for _, problem := range problems {
		if !strings.Contains(err.Error(), problem) {
			t.Errorf("Expected the %s problem to be reported, got %v", problem, err)
		}
	}

	if errors.Is(err, ErrNonConformant) {
		t.Error("Expected a configuration error, not a message violation")
	}
}
