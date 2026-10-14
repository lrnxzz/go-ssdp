package ssdp

import (
	"maps"
	"net/http"
	"net/netip"
	"slices"
	"time"
)

type EventType int

const (
	EventAppeared EventType = iota + 1
	EventChanged
	EventRebooted
	EventLeft
	EventExpired
)

func (t EventType) String() string {
	switch t {
	case EventAppeared:
		return "appeared"
	case EventChanged:
		return "changed"
	case EventRebooted:
		return "rebooted"
	case EventLeft:
		return "left"
	case EventExpired:
		return "expired"
	default:
		return "unknown"
	}
}

// Presence is what a control point knows about one device.
type Presence struct {
	Uuid       string
	Targets    []Target
	Location   string
	Server     ProductTokens
	BootId     Optional[int]
	ConfigId   Optional[int]
	ExpiresAt  time.Time
	From       netip.AddrPort
	Interface  string
	Extensions http.Header
}

type Event struct {
	Type     EventType
	Presence Presence
}

type cache struct {
	watched Target
	devices map[string]cacheEntry
}

type cacheEntry struct {
	presence Presence
	targets  map[string]cachedTarget
}

type cachedTarget struct {
	target    Target
	expiresAt time.Time
}

func newCache(watched Target) *cache {
	return &cache{
		watched: watched,
		devices: make(map[string]cacheEntry),
	}
}

func (c *cache) apply(datagram received, now time.Time) []Event {
	switch message := datagram.message.(type) {
	case SearchResponse:
		return c.see(message.Target, message.MaxAge, now, Presence{
			Uuid:       message.ServiceName.Uuid(),
			Location:   message.Location,
			Server:     message.Server,
			BootId:     message.BootId,
			ConfigId:   message.ConfigId,
			From:       datagram.from,
			Interface:  datagram.interfaceName,
			Extensions: message.Extensions,
		})
	case Alive:
		return c.see(message.Target, message.MaxAge, now, Presence{
			Uuid:       message.ServiceName.Uuid(),
			Location:   message.Location,
			Server:     message.Server,
			BootId:     message.BootId,
			ConfigId:   message.ConfigId,
			From:       datagram.from,
			Interface:  datagram.interfaceName,
			Extensions: message.Extensions,
		})
	case ByeBye:
		return c.leave(message.ServiceName.Uuid())
	case Update:
		return c.update(message)
	default:
		return nil
	}
}

func (c *cache) see(target Target, maxAge time.Duration, now time.Time, observed Presence) []Event {
	if !target.Answers(c.watched) {
		return nil
	}

	if maxAge <= 0 {
		maxAge = maxAgeFloor
	}

	entry, known := c.devices[observed.Uuid]
	eventType, changed := compare(entry.presence, observed, known)

	if !known || eventType == EventRebooted {
		entry = cacheEntry{
			targets: make(map[string]cachedTarget),
		}
	}

	entry.targets[target.String()] = cachedTarget{
		target:    target,
		expiresAt: now.Add(maxAge),
	}

	entry.presence = entry.summarize(observed)
	c.devices[observed.Uuid] = entry

	if !changed {
		return nil
	}

	return []Event{
		newEvent(eventType, entry.presence),
	}
}

func compare(previous, observed Presence, known bool) (EventType, bool) {
	switch {
	case !known:
		return EventAppeared, true
	case bootIdChanged(previous.BootId, observed.BootId):
		return EventRebooted, true
	case previous.Location != observed.Location,
		previous.Server.String() != observed.Server.String(),
		previous.ConfigId != observed.ConfigId:
		return EventChanged, true
	default:
		return 0, false
	}
}

func bootIdChanged(previous, observed Optional[int]) bool {
	before, hadBefore := previous.Get()
	after, hasAfter := observed.Get()

	return hadBefore && hasAfter && before != after
}

func (c *cache) leave(uuid string) []Event {
	entry, known := c.devices[uuid]
	if !known {
		return nil
	}

	delete(c.devices, uuid)

	return []Event{
		newEvent(EventLeft, entry.presence),
	}
}

func (c *cache) update(update Update) []Event {
	entry, known := c.devices[update.ServiceName.Uuid()]
	if !known {
		return nil
	}

	entry.presence.BootId = update.NextBootId

	if update.Location != "" {
		entry.presence.Location = update.Location
	}

	c.devices[update.ServiceName.Uuid()] = entry

	return []Event{
		newEvent(EventChanged, entry.presence),
	}
}

func (c *cache) expire(now time.Time) []Event {
	events := make([]Event, 0)

	for _, uuid := range slices.Sorted(maps.Keys(c.devices)) {
		entry := c.devices[uuid]

		maps.DeleteFunc(entry.targets, func(_ string, cached cachedTarget) bool {
			return !cached.expiresAt.After(now)
		})

		if len(entry.targets) > 0 {
			entry.presence = entry.summarize(entry.presence)
			c.devices[uuid] = entry

			continue
		}

		delete(c.devices, uuid)
		events = append(events, newEvent(EventExpired, entry.presence))
	}

	return events
}

func (c *cache) nextExpiry() (time.Time, bool) {
	var (
		next  time.Time
		found bool
	)

	for _, entry := range c.devices {
		for _, cached := range entry.targets {
			if !found || cached.expiresAt.Before(next) {
				next = cached.expiresAt
				found = true
			}
		}
	}

	return next, found
}

func (e cacheEntry) summarize(presence Presence) Presence {
	presence.Targets = make([]Target, 0, len(e.targets))
	presence.ExpiresAt = time.Time{}

	for _, key := range slices.Sorted(maps.Keys(e.targets)) {
		cached := e.targets[key]
		presence.Targets = append(presence.Targets, cached.target)

		if cached.expiresAt.After(presence.ExpiresAt) {
			presence.ExpiresAt = cached.expiresAt
		}
	}

	return presence
}

func newEvent(eventType EventType, presence Presence) Event {
	return Event{
		Type:     eventType,
		Presence: presence,
	}
}
