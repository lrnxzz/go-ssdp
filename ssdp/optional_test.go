package ssdp

import "testing"

func Test_Optional_AbsentByDefault(t *testing.T) {
	var counter Optional[int]

	if value, ok := counter.Get(); ok || value != 0 || counter.IsPresent() {
		t.Errorf("Expected an absent value, got %d, %t", value, ok)
	}
}

func Test_Optional_ZeroIsStillPresent(t *testing.T) {
	counter := NewOptional(0)

	if value, ok := counter.Get(); !ok || value != 0 || !counter.IsPresent() {
		t.Errorf("Expected a present zero, got %d, %t", value, ok)
	}
}

func Test_Optional_ComparesByValueAndPresence(t *testing.T) {
	first := NewOptional(7)
	second := NewOptional(7)

	if first != second {
		t.Error("Expected equal values to compare equal")
	}

	if NewOptional(0) == (Optional[int]{}) {
		t.Error("Expected a present zero to differ from an absent value")
	}
}
