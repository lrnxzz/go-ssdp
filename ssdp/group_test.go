package ssdp

import (
	"testing"
)

func Test_NewGroup(t *testing.T) {
	group, err := NewGroup(MulticastAddress, DefaultTtl)
	if err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}

	if group.String() != MulticastAddress || group.Ttl() != DefaultTtl || group.Address().Port() != 1900 {
		t.Errorf("Expected %s with TTL %d, got %s with %d", MulticastAddress, DefaultTtl, group, group.Ttl())
	}
}

func Test_NewGroup_RejectsWhatIsNotAnIpv4MulticastGroup(t *testing.T) {
	addresses := []string{
		"192.0.2.1:1900",
		"239.255.255.250",
		"239.255.255.250:0",
		"[ff02::c]:1900",
		"nonsense",
		"",
	}

	for _, address := range addresses {
		if _, err := NewGroup(address, DefaultTtl); err == nil {
			t.Errorf("Expected %q to be refused", address)
		}
	}
}

func Test_NewGroup_RejectsTtlOutsideAByte(t *testing.T) {
	tests := map[int]bool{
		-1:  false,
		0:   true,
		2:   true,
		255: true,
		256: false,
	}

	for ttl, accepted := range tests {
		if _, err := NewGroup(MulticastAddress, ttl); (err == nil) != accepted {
			t.Errorf("Expected TTL %d accepted = %t, got %v", ttl, accepted, err)
		}
	}
}

func Test_DefaultGroup(t *testing.T) {
	if DefaultGroup.String() != MulticastAddress || DefaultGroup.Ttl() != DefaultTtl {
		t.Errorf("Expected %s with TTL %d, got %s with %d", MulticastAddress, DefaultTtl, DefaultGroup, DefaultGroup.Ttl())
	}
}

func Test_Group_OrDefault(t *testing.T) {
	if got := (Group{}).orDefault(); got != DefaultGroup {
		t.Errorf("Expected the zero group to become %s, got %s", DefaultGroup, got)
	}

	group, err := NewGroup("239.255.255.250:54321", HostOnlyTtl)
	if err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}

	if got := group.orDefault(); got != group {
		t.Errorf("Expected %s to stay, got %s", group, got)
	}

	if !(Group{}).IsZero() || group.IsZero() {
		t.Error("Expected only the zero group to be zero")
	}
}
