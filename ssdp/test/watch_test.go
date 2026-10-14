package test

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/lrnxzz/go-ssdp/ssdp"
)

const (
	watchTimeout = 30 * time.Second
	eventBuffer  = 64
)

func Test_Integration_Watch_SeesADeviceAppearAndLeave(t *testing.T) {
	for name, initialize := range servers() {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), watchTimeout)
			defer cancel()

			container, address, err := initialize(t)
			if err != nil {
				t.Fatalf("Failed to initialize %s container: %v", name, err)
			}

			events := watch(t, ctx, mustTarget(t, mediaServer))

			appeared := expectEvent(t, events, ssdp.EventAppeared, address)

			if appeared.Presence.Location == "" || appeared.Presence.ExpiresAt.Before(time.Now()) {
				t.Errorf("Expected %s present with a location and an expiry ahead, got %+v", name, appeared.Presence)
			}

			if err := container.Terminate(ctx); err != nil {
				t.Fatalf("Failed to terminate %s container: %v", name, err)
			}

			left := expectEvent(t, events, ssdp.EventLeft, address)

			if left.Presence.Uuid != appeared.Presence.Uuid {
				t.Errorf("Expected %s to leave, got %s", appeared.Presence.Uuid, left.Presence.Uuid)
			}
		})
	}
}

func watch(t *testing.T, ctx context.Context, target ssdp.Target) <-chan ssdp.Event {
	t.Helper()

	events := make(chan ssdp.Event, eventBuffer)

	go func() {
		defer close(events)

		for event, err := range newControlPoint(t).Watch(ctx, target) {
			if err != nil {
				t.Errorf("Failed to watch: %v", err)

				return
			}

			select {
			case events <- event:
			case <-ctx.Done():
				return
			}
		}
	}()

	return events
}

func expectEvent(t *testing.T, events <-chan ssdp.Event, eventType ssdp.EventType, address netip.Addr) ssdp.Event {
	t.Helper()

	for event := range events {
		if event.Type == eventType && event.Presence.From.Addr() == address {
			return event
		}
	}

	t.Fatalf("Expected a %s event from %s before the watch ended", eventType, address)

	return ssdp.Event{}
}
