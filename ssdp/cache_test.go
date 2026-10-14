package ssdp

import (
	"net/netip"
	"testing"
	"time"
)

var cacheNow = time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)

func Test_Cache_AnnouncesADeviceOnce(t *testing.T) {
	devices := newCache(newAllTarget())

	expectEvents(t, devices.apply(aliveFrom(t, "a", exampleDevice, 1, exampleLocation), cacheNow), EventAppeared)
	expectEvents(t, devices.apply(aliveFrom(t, "a", exampleDevice, 1, exampleLocation), cacheNow.Add(time.Minute)))
}

func Test_Cache_ReportsWhatChanged(t *testing.T) {
	devices := newCache(newAllTarget())
	devices.apply(aliveFrom(t, "a", exampleDevice, 1, exampleLocation), cacheNow)

	events := devices.apply(aliveFrom(t, "a", exampleDevice, 1, "http://192.0.2.21:49152/description.xml"), cacheNow)

	expectEvents(t, events, EventChanged)

	if events[0].Presence.Location != "http://192.0.2.21:49152/description.xml" {
		t.Errorf("Expected the new location, got %s", events[0].Presence.Location)
	}
}

func Test_Cache_TreatsADifferentBootIdAsAReboot(t *testing.T) {
	devices := newCache(newAllTarget())
	devices.apply(aliveFrom(t, "a", exampleDevice, 5, exampleLocation), cacheNow)
	devices.apply(aliveFrom(t, "a", "upnp:rootdevice", 5, exampleLocation), cacheNow)

	events := devices.apply(aliveFrom(t, "a", exampleDevice, 6, exampleLocation), cacheNow)
	expectEvents(t, events, EventRebooted)

	if len(events[0].Presence.Targets) != 1 {
		t.Errorf("Expected the reboot to start over with one target, got %v", events[0].Presence.Targets)
	}

	expectEvents(t, devices.apply(aliveFrom(t, "a", exampleDevice, 4, exampleLocation), cacheNow), EventRebooted)
}

func Test_Cache_NeverSeesARebootWithoutBootId(t *testing.T) {
	devices := newCache(newAllTarget())
	devices.apply(aliveFrom(t, "a", exampleDevice, -1, exampleLocation), cacheNow)

	expectEvents(t, devices.apply(aliveFrom(t, "a", exampleDevice, 3, exampleLocation), cacheNow))
}

func Test_Cache_ForgetsADeviceOnByeBye(t *testing.T) {
	devices := newCache(mustTarget(t, exampleDevice))
	devices.apply(aliveFrom(t, "a", exampleDevice, 1, exampleLocation), cacheNow)

	events := devices.apply(byeByeFrom("a", newRootDeviceTarget()), cacheNow)
	expectEvents(t, events, EventLeft)

	if events[0].Presence.Uuid != "a" {
		t.Errorf("Expected device a to leave, got %s", events[0].Presence.Uuid)
	}

	if _, ok := devices.nextExpiry(); ok {
		t.Error("Expected nothing left to expire")
	}
}

func Test_Cache_IgnoresByeByeOfUnknownDevices(t *testing.T) {
	devices := newCache(newAllTarget())

	expectEvents(t, devices.apply(byeByeFrom("a", newRootDeviceTarget()), cacheNow))
}

func Test_Cache_UpdateMovesTheBootIdForward(t *testing.T) {
	devices := newCache(newAllTarget())
	devices.apply(aliveFrom(t, "a", exampleDevice, 1, exampleLocation), cacheNow)

	update := Update{
		Host:        MulticastAddress,
		Target:      mustTarget(t, exampleDevice),
		ServiceName: newServiceName("a", mustTarget(t, exampleDevice)),
		Location:    exampleLocation,
		BootId:      NewOptional(1),
		ConfigId:    NewOptional(7),
		NextBootId:  NewOptional(2),
	}

	events := devices.apply(receivedOf(update), cacheNow)
	expectEvents(t, events, EventChanged)

	if bootId, _ := events[0].Presence.BootId.Get(); bootId != 2 {
		t.Errorf("Expected boot id 2, got %d", bootId)
	}

	expectEvents(t, devices.apply(aliveFrom(t, "a", exampleDevice, 2, exampleLocation), cacheNow))
	expectEvents(t, devices.apply(receivedOf(Update{
		ServiceName: newServiceName("unknown", newRootDeviceTarget()),
	}), cacheNow))
}

func Test_Cache_ExpiresOnlyWhenEveryAdvertisementExpired(t *testing.T) {
	devices := newCache(newAllTarget())

	shortLived := aliveFrom(t, "a", "upnp:rootdevice", 1, exampleLocation)
	shortLived.message = withMaxAge(shortLived.message, time.Minute)

	longLived := aliveFrom(t, "a", exampleDevice, 1, exampleLocation)
	longLived.message = withMaxAge(longLived.message, time.Hour)

	devices.apply(shortLived, cacheNow)
	devices.apply(longLived, cacheNow)

	if next, _ := devices.nextExpiry(); !next.Equal(cacheNow.Add(time.Minute)) {
		t.Errorf("Expected the next expiry in a minute, got %s", next)
	}

	expectEvents(t, devices.expire(cacheNow.Add(time.Minute)))

	if next, _ := devices.nextExpiry(); !next.Equal(cacheNow.Add(time.Hour)) {
		t.Errorf("Expected the next expiry in an hour, got %s", next)
	}

	events := devices.expire(cacheNow.Add(time.Hour))
	expectEvents(t, events, EventExpired)

	if events[0].Presence.Uuid != "a" {
		t.Errorf("Expected device a to expire, got %s", events[0].Presence.Uuid)
	}
}

func Test_Cache_KeepsTheLatestExpiry(t *testing.T) {
	devices := newCache(newAllTarget())

	alive := aliveFrom(t, "a", exampleDevice, 1, exampleLocation)
	alive.message = withMaxAge(alive.message, time.Hour)

	events := devices.apply(alive, cacheNow)

	if expiresAt := events[0].Presence.ExpiresAt; !expiresAt.Equal(cacheNow.Add(time.Hour)) {
		t.Errorf("Expected the device to expire in an hour, got %s", expiresAt)
	}
}

func Test_Cache_GivesMissingMaxAgeTheFloor(t *testing.T) {
	devices := newCache(newAllTarget())

	alive := aliveFrom(t, "a", exampleDevice, 1, exampleLocation)
	alive.message = withMaxAge(alive.message, 0)
	devices.apply(alive, cacheNow)

	if next, _ := devices.nextExpiry(); !next.Equal(cacheNow.Add(maxAgeFloor)) {
		t.Errorf("Expected the expiry %s away, got %s", maxAgeFloor, next.Sub(cacheNow))
	}
}

func Test_Cache_IgnoresTargetsItDoesNotWatch(t *testing.T) {
	devices := newCache(newVendorTarget(exampleVendor))

	expectEvents(t, devices.apply(aliveFrom(t, "a", exampleDevice, 1, exampleLocation), cacheNow))
	expectEvents(t, devices.apply(aliveFrom(t, "vendor", exampleVendor, -1, "http://192.0.2.10:8080/"), cacheNow), EventAppeared)
}

func Test_Cache_RecognizesNewerVersionsOfTheWatchedType(t *testing.T) {
	devices := newCache(mustTarget(t, exampleDevice))

	expectEvents(t, devices.apply(aliveFrom(t, "a", "urn:schemas-upnp-org:device:MediaRenderer:2", 1, exampleLocation), cacheNow), EventAppeared)
}

func Test_Cache_CountsSearchResponsesAsSightings(t *testing.T) {
	devices := newCache(newVendorTarget(exampleVendor))

	response := SearchResponse{
		Target:      newVendorTarget(exampleVendor),
		ServiceName: newServiceName("vendor:serial:X1", Target{}),
		Location:    "http://192.0.2.10:8080/",
		MaxAge:      time.Hour,
	}

	events := devices.apply(receivedOf(response), cacheNow)
	expectEvents(t, events, EventAppeared)

	if events[0].Presence.Uuid != "vendor:serial:X1" || events[0].Presence.Location != "http://192.0.2.10:8080/" {
		t.Errorf("Expected the vendor device at its location, got %+v", events[0].Presence)
	}
}

func Test_Cache_IgnoresSearches(t *testing.T) {
	devices := newCache(newAllTarget())

	search := SearchRequest{
		Host:    MulticastAddress,
		Target:  newAllTarget(),
		MaxWait: time.Second,
	}

	expectEvents(t, devices.apply(receivedOf(search), cacheNow))
}

func Test_EventType_String(t *testing.T) {
	tests := map[EventType]string{
		EventAppeared: "appeared",
		EventChanged:  "changed",
		EventRebooted: "rebooted",
		EventLeft:     "left",
		EventExpired:  "expired",
		0:             "unknown",
	}

	for eventType, expected := range tests {
		if got := eventType.String(); got != expected {
			t.Errorf("Expected %q, got %q", expected, got)
		}
	}
}

func aliveFrom(t *testing.T, uuid, target string, bootId int, location string) received {
	t.Helper()

	alive := Alive{
		Host:        MulticastAddress,
		Target:      mustTarget(t, target),
		ServiceName: newServiceName(uuid, mustTarget(t, target)),
		Location:    location,
		Server:      mustProductTokens(t, exampleServer),
		MaxAge:      maxAgeFloor,
	}

	if bootId >= 0 {
		alive.BootId = NewOptional(bootId)
	}

	return receivedOf(alive)
}

func byeByeFrom(uuid string, target Target) received {
	return receivedOf(ByeBye{
		Host:        MulticastAddress,
		Target:      target,
		ServiceName: newServiceName(uuid, target),
	})
}

func receivedOf(message Message) received {
	return received{
		message:       message,
		from:          netip.MustParseAddrPort("192.0.2.20:1900"),
		interfaceName: "eth0",
	}
}

func withMaxAge(message Message, maxAge time.Duration) Message {
	alive, ok := message.(Alive)
	if !ok {
		return message
	}

	alive.MaxAge = maxAge

	return alive
}

func expectEvents(t *testing.T, events []Event, expected ...EventType) {
	t.Helper()

	if len(events) != len(expected) {
		t.Fatalf("Expected events %v, got %v", expected, events)
	}

	for index, event := range events {
		if event.Type != expected[index] {
			t.Errorf("Expected event %d to be %s, got %s", index, expected[index], event.Type)
		}
	}
}
