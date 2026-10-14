package ssdp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func Test_ControlPointConfig_Validate(t *testing.T) {
	valid := map[string]ControlPointConfig{
		"empty": {},
		"UDA 1.1 user agent": {
			UserAgent: exampleServer,
		},
		"UDA 2.0 user agent with a name": {
			UserAgent: "unix/5.1 UPnP/2.0 MyProduct/1.0",
			Name:      "Living room",
		},
	}

	for name, config := range valid {
		if err := config.Validate(); err != nil {
			t.Errorf("Expected %s to be valid, got %v", name, err)
		}
	}

	invalid := map[string]ControlPointConfig{
		"user agent without UPnP": {
			UserAgent: "Linux/6.6 Product/1.0",
		},
		"UDA 2.0 user agent without a name": {
			UserAgent: "unix/5.1 UPnP/2.0 MyProduct/1.0",
		},
	}

	for name, config := range invalid {
		if err := config.Validate(); err == nil {
			t.Errorf("Expected %s to be refused", name)
		}
	}
}

func Test_NewControlPoint_FillsTheDefaults(t *testing.T) {
	created := newControlPoint(ControlPointConfig{})

	if created.transport.group != DefaultGroup || !created.userAgent.Upnp().AtLeast(Version11) || created.userAgent.Product().Name() != "go-ssdp" {
		t.Errorf("Expected the default group and a go-ssdp UDA 1.1 user agent, got %s and %s", created.transport.group, created.userAgent)
	}
}

func Test_Search_RoundsMaxWaitIntoMx(t *testing.T) {
	tests := map[time.Duration]time.Duration{
		0:                       time.Second,
		1500 * time.Millisecond: 2 * time.Second,
		10 * time.Second:        5 * time.Second,
	}

	for maxWait, expected := range tests {
		group := hostOnlyGroup(t)
		searcher := newControlPoint(ControlPointConfig{
			Group: group,
		})

		current := newTransport(group, nil)
		listener, _ := openEndpoints(t, current)

		ctx, cancel := context.WithCancel(t.Context())
		messages := current.listen(ctx, listener)

		go func() {
			for range searcher.Search(ctx, "ssdp:all", maxWait) {
			}
		}()

		request, ok := receive(t, messages).message.(SearchRequest)
		cancel()

		if !ok || request.MaxWait != expected {
			t.Errorf("Expected MX %s for %s, got %+v", expected, maxWait, request)
		}
	}
}

func Test_Search_YieldsEachServiceOnceWithWhereItCameFrom(t *testing.T) {
	group := hostOnlyGroup(t)

	announced, err := NewDevice(DeviceConfig{
		Group: group,
		Uuid:  exampleUuid,
		Targets: []string{
			"upnp:rootdevice",
		},
		Location: exampleLocation,
	})
	if err != nil {
		t.Fatalf("Failed to create device: %v", err)
	}
	defer announced.Close()

	searcher, err := NewControlPoint(ControlPointConfig{
		Group: group,
	})
	if err != nil {
		t.Fatalf("Failed to create control point: %v", err)
	}

	responses := make([]SearchResponse, 0)

	for response, err := range searcher.Search(t.Context(), "upnp:rootdevice", time.Second) {
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}

		responses = append(responses, response)
	}

	if len(responses) != 1 {
		t.Fatalf("Expected one answer for the one service, got %d", len(responses))
	}

	if !responses[0].From.IsValid() || responses[0].Interface == "" {
		t.Errorf("Expected the address and interface the answer came through, got %+v", responses[0])
	}
}

func Test_Search_RefusesATargetThatDoesNotParse(t *testing.T) {
	values := 0

	for _, err := range newControlPoint(ControlPointConfig{}).Search(t.Context(), "", time.Second) {
		values++

		if !errors.Is(err, ErrMalformed) {
			t.Errorf("Expected a malformed target, got %v", err)
		}
	}

	if values != 1 {
		t.Errorf("Expected the error to end the search, got %d values", values)
	}
}

func Test_Search_EndsWithTheErrorOfItsContext(t *testing.T) {
	searcher := newControlPoint(ControlPointConfig{
		Group: hostOnlyGroup(t),
	})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var last error

	for _, err := range searcher.Search(ctx, "ssdp:all", time.Second) {
		last = err
	}

	if !errors.Is(last, context.Canceled) {
		t.Errorf("Expected the search to end with the canceled context, got %v", last)
	}
}

func Test_ControlPoint_SearchesConcurrently(t *testing.T) {
	searcher := newControlPoint(ControlPointConfig{
		Group: hostOnlyGroup(t),
	})

	var searches sync.WaitGroup

	for range 4 {
		searches.Go(func() {
			for _, err := range searcher.Search(t.Context(), "ssdp:all", time.Second) {
				if err != nil {
					t.Errorf("Failed to search: %v", err)
				}
			}
		})
	}

	searches.Wait()
}
