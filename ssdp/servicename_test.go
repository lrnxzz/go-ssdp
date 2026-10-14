package ssdp

import (
	"errors"
	"testing"
)

func Test_ParseServiceName(t *testing.T) {
	tests := map[string]string{
		"uuid:" + exampleUuid + "::upnp:rootdevice":  exampleUuid + " | upnp:rootdevice",
		"uuid:" + exampleUuid:                        exampleUuid + " | ",
		"uuid:" + exampleUuid + "::" + exampleDevice: exampleUuid + " | " + exampleDevice,

		"uuid:vendor:serial:X1": "vendor:serial:X1 | ",
	}

	for value, expected := range tests {
		serviceName, err := ParseServiceName(value)
		if err != nil {
			t.Errorf("Failed to parse %q: %v", value, err)

			continue
		}

		target, _ := serviceName.Target()

		if got := serviceName.Uuid() + " | " + target.String(); got != expected {
			t.Errorf("Expected ParseServiceName(%q) = %q, got %q", value, expected, got)
		}

		if serviceName.String() != value {
			t.Errorf("Expected %q to print back unchanged, got %q", value, serviceName)
		}
	}
}

func Test_ParseServiceName_RejectsWhatIsNotAUsn(t *testing.T) {
	values := []string{
		"",
		exampleUuid,
		"uuid:",
		"uuid:::upnp:rootdevice",
		"uuid:" + exampleUuid + "::",
		"UUID:" + exampleUuid,
	}

	for _, value := range values {
		if _, err := ParseServiceName(value); !errors.Is(err, ErrMalformed) {
			t.Errorf("Expected ParseServiceName(%q) to be malformed, got %v", value, err)
		}
	}
}

func Test_NewServiceName_LeavesTheUuidTargetOut(t *testing.T) {
	serviceName := NewServiceName(exampleUuid, NewUuidTarget(exampleUuid))

	if got := serviceName.String(); got != "uuid:"+exampleUuid {
		t.Errorf("Expected uuid:%s, got %s", exampleUuid, got)
	}

	if _, ok := serviceName.Target(); ok {
		t.Error("Expected a bare uuid to have no target")
	}
}

func Test_NewServiceName_JoinsUuidAndTarget(t *testing.T) {
	serviceName := NewServiceName(exampleUuid, NewRootDeviceTarget())

	if got := serviceName.String(); got != "uuid:"+exampleUuid+"::upnp:rootdevice" {
		t.Errorf("Expected uuid:%s::upnp:rootdevice, got %s", exampleUuid, got)
	}

	target, ok := serviceName.Target()
	if !ok || target.Type() != TargetRootDevice {
		t.Errorf("Expected the root device target, got %s", target)
	}
}

func Test_ServiceName_IsZero(t *testing.T) {
	if !(ServiceName{}).IsZero() || NewServiceName(exampleUuid, Target{}).IsZero() {
		t.Error("Expected only the zero service name to be zero")
	}
}
