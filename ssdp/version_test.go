package ssdp

import (
	"errors"
	"strings"
	"testing"
)

func Test_ParseVersion(t *testing.T) {
	tests := map[string]Version{
		"1.0":  Version10,
		"1.1":  Version11,
		"2.0":  Version20,
		"2.10": newVersion(2, 10),
	}

	for value, expected := range tests {
		got, err := ParseVersion(value)
		if err != nil {
			t.Errorf("Failed to parse %q: %v", value, err)

			continue
		}

		if got != expected {
			t.Errorf("Expected ParseVersion(%q) = %s, got %s", value, expected, got)
		}
	}
}

func Test_ParseVersion_RejectsWhatIsNotMajorMinor(t *testing.T) {
	values := []string{
		"",
		"1",
		"1.",
		".1",
		"a.b",
		"0.9",
		"1.-1",
		"1.1.1",
		"v1.1",
	}

	for _, value := range values {
		if _, err := ParseVersion(value); !errors.Is(err, ErrMalformed) {
			t.Errorf("Expected ParseVersion(%q) to be malformed, got %v", value, err)
		}
	}
}

func Test_Version_AtLeast(t *testing.T) {
	tests := map[string]bool{
		"1.1 1.0":   true,
		"1.0 1.1":   false,
		"1.1 1.1":   true,
		"2.0 1.1":   true,
		"1.1 2.0":   false,
		"2.10 2.2":  true,
		"2.2 2.10":  false,
		"3.0 2.99":  true,
		"1.99 2.0":  false,
		"2.0 1.999": true,
	}

	for pair, expected := range tests {
		left, right, _ := strings.Cut(pair, " ")

		version, err := ParseVersion(left)
		if err != nil {
			t.Fatalf("Failed to parse %q: %v", left, err)
		}

		other, err := ParseVersion(right)
		if err != nil {
			t.Fatalf("Failed to parse %q: %v", right, err)
		}

		if got := version.AtLeast(other); got != expected {
			t.Errorf("Expected %s.AtLeast(%s) = %t, got %t", version, other, expected, got)
		}
	}
}

func Test_Version_String(t *testing.T) {
	tests := map[Version]string{
		Version10:         "1.0",
		Version11:         "1.1",
		newVersion(2, 10): "2.10",
	}

	for version, expected := range tests {
		if got := version.String(); got != expected {
			t.Errorf("Expected %q, got %q", expected, got)
		}
	}
}

func Test_Version_ParsesWhatItPrints(t *testing.T) {
	versions := []Version{
		Version10,
		Version11,
		Version20,
		newVersion(2, 10),
	}

	for _, version := range versions {
		parsed, err := ParseVersion(version.String())
		if err != nil || parsed != version {
			t.Errorf("Expected %s to parse back to itself, got %s, %v", version, parsed, err)
		}
	}
}

func Test_Version_AMessageWithoutVersionFollowsUda10(t *testing.T) {
	if got := (Version{}).rules(); got != Version10 {
		t.Errorf("Expected no version to follow UDA 1.0, got %s", got)
	}

	if got := Version11.rules(); got != Version11 {
		t.Errorf("Expected UDA 1.1 to keep its own rules, got %s", got)
	}

	if !(Version{}).IsZero() || Version10.IsZero() {
		t.Error("Expected only the zero version to be zero")
	}
}

func Test_Version_Parts(t *testing.T) {
	version := newVersion(2, 10)

	if version.Major() != 2 || version.Minor() != 10 {
		t.Errorf("Expected 2 and 10, got %d and %d", version.Major(), version.Minor())
	}
}
