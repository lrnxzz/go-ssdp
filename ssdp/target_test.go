package ssdp

import (
	"errors"
	"strings"
	"testing"
)

func Test_ParseTarget_TellsTheFormsApart(t *testing.T) {
	tests := map[string]TargetType{
		"ssdp:all":            TargetAll,
		"upnp:rootdevice":     TargetRootDevice,
		"uuid:" + exampleUuid: TargetUuid,
		exampleDevice:         TargetDeviceType,
		"urn:schemas-upnp-org:service:AVTransport:2": TargetServiceType,
		"urn:dial-multiscreen-org:service:dial:1":    TargetServiceType,
		"urn:example-com:device:player:1":            TargetDeviceType,
		exampleVendor:                                TargetVendor,
		"urn:schemas-upnp-org:device:MediaRenderer":  TargetVendor,
		"urn:example-com:widget:Gadget:1":            TargetVendor,
		"urn:example-com:device:Gadget:one":          TargetVendor,
		"urn:example-com:device:Gadget:0":            TargetVendor,
		"urn::device:Gadget:1":                       TargetVendor,
	}

	for value, expected := range tests {
		target, err := ParseTarget(value)
		if err != nil {
			t.Errorf("Failed to parse %q: %v", value, err)

			continue
		}

		if target.Type() != expected {
			t.Errorf("Expected %q to be a %s target, got %s", value, expected, target.Type())
		}

		if target.String() != value {
			t.Errorf("Expected %q to print back unchanged, got %q", value, target)
		}
	}
}

func Test_ParseTarget_RejectsEmptyTargets(t *testing.T) {
	values := []string{
		"",
		"uuid:",
	}

	for _, value := range values {
		if _, err := ParseTarget(value); !errors.Is(err, ErrMalformed) {
			t.Errorf("Expected ParseTarget(%q) to be malformed, got %v", value, err)
		}
	}
}

func Test_ParseTarget_ReadsTheTypeParts(t *testing.T) {
	target := mustTarget(t, "urn:schemas-upnp-org:service:AVTransport:2")

	if target.Domain() != standardDomain || target.Name() != "AVTransport" || target.Version() != 2 {
		t.Errorf("Expected %s AVTransport 2, got %s %s %d", standardDomain, target.Domain(), target.Name(), target.Version())
	}
}

func Test_ParseTarget_ReadsTheUuid(t *testing.T) {
	if got := mustTarget(t, "uuid:"+exampleUuid).Name(); got != exampleUuid {
		t.Errorf("Expected uuid %s, got %s", exampleUuid, got)
	}
}

func Test_NewServiceTypeTarget_ReplacesPeriodsInTheDomain(t *testing.T) {
	target, err := newServiceTypeTarget("dial-multiscreen.org", "dial", 1)
	if err != nil {
		t.Fatalf("Failed to create target: %v", err)
	}

	if got := target.String(); got != "urn:dial-multiscreen-org:service:dial:1" {
		t.Errorf("Expected urn:dial-multiscreen-org:service:dial:1, got %s", got)
	}
}

func Test_NewDeviceTypeTarget_BuildsTheUrn(t *testing.T) {
	target, err := newDeviceTypeTarget(standardDomain, "MediaRenderer", 1)
	if err != nil {
		t.Fatalf("Failed to create target: %v", err)
	}

	if target.String() != exampleDevice || target.Type() != TargetDeviceType {
		t.Errorf("Expected device type %s, got %s %s", exampleDevice, target.Type(), target)
	}

	if parsed := mustTarget(t, exampleDevice); parsed != target {
		t.Errorf("Expected the parsed target to equal the built one, got %+v and %+v", parsed, target)
	}
}

func Test_NewDeviceTypeTarget_RejectsInvalidTypes(t *testing.T) {
	longest := strings.Repeat("x", 64)

	if _, err := newDeviceTypeTarget(standardDomain, longest, 1); err != nil {
		t.Errorf("Expected a 64 character name to be accepted, got %v", err)
	}

	tests := map[string]error{
		"empty domain":  typeTargetError("", "Gadget", 1),
		"empty name":    typeTargetError(standardDomain, "", 1),
		"long name":     typeTargetError(standardDomain, longest+"x", 1),
		"version zero":  typeTargetError(standardDomain, "Gadget", 0),
		"version below": typeTargetError(standardDomain, "Gadget", -1),
	}

	for name, err := range tests {
		if !errors.Is(err, ErrMalformed) {
			t.Errorf("Expected %s to be malformed, got %v", name, err)
		}
	}
}

func typeTargetError(domain, name string, version int) error {
	_, err := newDeviceTypeTarget(domain, name, version)

	return err
}

func Test_Target_Answers(t *testing.T) {
	tests := map[string]bool{
		"upnp:rootdevice ssdp:all": true,
		"vendor:service ssdp:all":  true,
		"uuid:a ssdp:all":          true,

		"urn:schemas-upnp-org:device:MediaRenderer:2 urn:schemas-upnp-org:device:MediaRenderer:1": true,
		"urn:schemas-upnp-org:device:MediaRenderer:2 urn:schemas-upnp-org:device:MediaRenderer:2": true,
		"urn:schemas-upnp-org:device:MediaRenderer:1 urn:schemas-upnp-org:device:MediaRenderer:2": false,

		"urn:schemas-upnp-org:device:MediaRenderer:1 urn:schemas-upnp-org:device:MediaServer:1": false,
		"urn:schemas-upnp-org:device:Gadget:1 urn:example-com:device:Gadget:1":                  false,
		"urn:schemas-upnp-org:service:Gadget:1 urn:schemas-upnp-org:device:Gadget:1":            false,

		"upnp:rootdevice upnp:rootdevice": true,
		"uuid:a uuid:a":                   true,
		"uuid:a uuid:b":                   false,
		"uuid:a upnp:rootdevice":          false,
		"vendor:service vendor:service":   true,
		"vendor:service vendor:other":     false,
		"vendor:service upnp:rootdevice":  false,
	}

	for pair, expected := range tests {
		announced, searched, _ := strings.Cut(pair, " ")

		if got := mustTarget(t, announced).Answers(mustTarget(t, searched)); got != expected {
			t.Errorf("Expected %s answering %s = %t, got %t", announced, searched, expected, got)
		}
	}
}

func Test_Target_ResponseTarget(t *testing.T) {
	tests := map[string]string{
		"urn:schemas-upnp-org:device:MediaRenderer:2 urn:schemas-upnp-org:device:MediaRenderer:1": "urn:schemas-upnp-org:device:MediaRenderer:1",
		"urn:schemas-upnp-org:device:MediaRenderer:2 ssdp:all":                                    "urn:schemas-upnp-org:device:MediaRenderer:2",
		"upnp:rootdevice ssdp:all":      "upnp:rootdevice",
		"vendor:service vendor:service": "vendor:service",
		"uuid:a uuid:a":                 "uuid:a",
	}

	for pair, expected := range tests {
		announced, searched, _ := strings.Cut(pair, " ")

		if got := mustTarget(t, announced).responseTarget(mustTarget(t, searched)).String(); got != expected {
			t.Errorf("Expected %s answering %s with ST %s, got %s", announced, searched, expected, got)
		}
	}
}

func Test_Target_IsZero(t *testing.T) {
	if !(Target{}).IsZero() || newAllTarget().IsZero() {
		t.Error("Expected only the zero target to be zero")
	}
}

func Test_TargetType_String(t *testing.T) {
	tests := map[TargetType]string{
		TargetAll:         "all",
		TargetRootDevice:  "root device",
		TargetUuid:        "uuid",
		TargetDeviceType:  "device type",
		TargetServiceType: "service type",
		TargetVendor:      "vendor",
		0:                 "unknown",
	}

	for targetType, expected := range tests {
		if got := targetType.String(); got != expected {
			t.Errorf("Expected %q, got %q", expected, got)
		}
	}
}
