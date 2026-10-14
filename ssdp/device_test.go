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

func Test_NewDevice_AnnouncesTheUuidAndEveryTarget(t *testing.T) {
	service := "urn:schemas-upnp-org:service:AVTransport:1"

	opened, err := NewDevice(DeviceConfig{
		Group: hostOnlyGroup(t),
		Uuid:  exampleUuid,
		Targets: []string{
			"upnp:rootdevice",
			exampleDevice,
			service,
			"uuid:" + exampleUuid,
		},
		Location: exampleLocation,
	})
	if err != nil {
		t.Fatalf("Failed to create device: %v", err)
	}
	defer opened.Close()

	expected := []string{
		"uuid:" + exampleUuid + " uuid:" + exampleUuid,
		"upnp:rootdevice uuid:" + exampleUuid + "::upnp:rootdevice",
		exampleDevice + " uuid:" + exampleUuid + "::" + exampleDevice,
		service + " uuid:" + exampleUuid + "::" + service,
	}

	advertisements := opened.(*device).advertisements

	if len(advertisements) != len(expected) {
		t.Fatalf("Expected %d advertisements, got %d", len(expected), len(advertisements))
	}

	for index, advertised := range advertisements {
		if got := advertised.target.String() + " " + advertised.serviceName.String(); got != expected[index] {
			t.Errorf("Expected advertisement %d to be %q, got %q", index, expected[index], got)
		}
	}
}

func Test_NewDevice_FillsTheDefaults(t *testing.T) {
	started := time.Now().Unix()

	opened, err := NewDevice(DeviceConfig{
		Group:    hostOnlyGroup(t),
		Uuid:     exampleUuid,
		Location: exampleLocation,
	})
	if err != nil {
		t.Fatalf("Failed to create device: %v", err)
	}
	defer opened.Close()

	created := opened.(*device)
	bootId, _ := created.bootId.Get()

	if !created.server.Upnp().AtLeast(Version11) || created.maxAge != maxAgeFloor || int64(bootId) < started {
		t.Errorf("Expected a UDA 1.1 server, a max age of %s and a boot id from now, got %s, %s and %d", maxAgeFloor, created.server, created.maxAge, bootId)
	}
}

func Test_Device_ClosesOnce(t *testing.T) {
	opened, err := NewDevice(DeviceConfig{
		Group:    hostOnlyGroup(t),
		Uuid:     exampleUuid,
		Location: exampleLocation,
	})
	if err != nil {
		t.Fatalf("Failed to create device: %v", err)
	}

	if err := opened.Close(); err != nil {
		t.Fatalf("Failed to close: %v", err)
	}

	if err := opened.Close(); err != nil {
		t.Errorf("Expected closing again to do nothing, got %v", err)
	}
}

func Test_DeviceConfig_Validate(t *testing.T) {
	valid := DeviceConfig{
		Uuid: exampleUuid,
		Targets: []string{
			"upnp:rootdevice",
			exampleDevice,
		},
		Location: exampleLocation,
		Server:   exampleServer,
		ConfigId: 1,
	}

	if err := valid.Validate(); err != nil {
		t.Errorf("Expected a valid configuration, got %v", err)
	}

	noUuid := valid
	noUuid.Uuid = ""

	searchAll := valid
	searchAll.Targets = []string{
		"ssdp:all",
	}

	emptyTarget := valid
	emptyTarget.Targets = []string{
		"",
	}

	relativeLocation := valid
	relativeLocation.Location = "/description.xml"

	serverWithoutUpnp := valid
	serverWithoutUpnp.Server = "Linux/6.6 Product/1.0"

	shortMaxAge := valid
	shortMaxAge.MaxAge = 500 * time.Millisecond

	largeConfigId := valid
	largeConfigId.ConfigId = configIdCeiling + 1

	tests := map[string]DeviceConfig{
		"no uuid":             noUuid,
		"ssdp:all":            searchAll,
		"empty target":        emptyTarget,
		"relative location":   relativeLocation,
		"server without UPnP": serverWithoutUpnp,
		"short max age":       shortMaxAge,
		"large config id":     largeConfigId,
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
		"uuid",
		"location",
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
