// Package ssdp implements the Simple Service Discovery Protocol, the discovery layer of UPnP: HTTP/1.1
// shaped messages over UDP multicast, sent by control points that search and by devices that answer
// and announce themselves.
//
// It follows chapter 1 of the UPnP Device Architecture. Section numbers, like §1.3.2, refer to UDA 1.1
// unless noted.
//
//   - [UDA 1.1]
//   - [UDA 2.0], which adds the control point fields
//   - [SSDP/1.0], the IETF draft the architecture builds on
//
// # Messages
//
//   - The five messages are [Search], [SearchResponse], [Alive], [ByeBye] and [Update].
//   - [Decode] tells them apart by the start-line (§1.1.1) and, for notifications, by NTS.
//   - Messages marshal in the field order of the specification's examples.
//   - Vendor fields (§1.1.3) are kept in Extensions.
//
// # Lenient decoding
//
// §1.1.5 lets receivers parse loosely to interoperate, and real devices need it:
//
//   - A datagram without the final blank line still decodes.
//   - Whitespace around values is trimmed, including what a folded line leaves (RFC 2616 §4.2).
//   - Field names match in any case (§1.1.2).
//   - CACHE-CONTROL reads "max-age = 1800" as the examples write it and ignores other directives (§1.2.2).
//   - Counters ignore leading zeros (§1.2.2).
//   - Uuids are not held to RFC 4122 (§1.1.4); some vendors put a serial number there.
//   - SERVER and USER-AGENT are read around their "UPnP/x.y" token, so UDA 1.0 commas and product
//     names with spaces still parse.
//
// # Conformance
//
// Check reports the MUSTs a message breaks as [Violations], under the version it declares in SERVER or
// USER-AGENT:
//
//   - No declared version means UDA 1.0 rules.
//   - UDA 1.1 announcements carry BOOTID.UPNP.ORG and CONFIGID.UPNP.ORG; answers only BOOTID (§1.3.3).
//   - UDA 2.0 control points name themselves in CPFN.UPNP.ORG (UDA 2.0 §1.3.2).
//   - A byebye has no SERVER, so its version can't be known.
//   - ssdp:update exists only since 1.1: its counters are required and NEXTBOOTID exceeds BOOTID (§1.2.4).
//   - A multicast search carries MX, and NT never names ssdp:all.
//
// # Targets
//
//   - A [Target] is ssdp:all, upnp:rootdevice, a uuid, a device or service type, or a vendor value.
//   - Periods in a vendor domain become hyphens (§1.2.2); type names have at most 64 characters (§2.3).
//   - A device answers searches for its type and every older version of it (§1.3.2).
//   - Answers echo the searched version; ssdp:all gets one answer per announced target (§1.3.3).
//
// # Transport
//
//   - Every IPv4 interface gets its own socket, bound to its address, so answers return on the
//     interface that searched and devices are found even behind a VPN or a virtual switch (§1.3).
//   - Loopback is left out; this host is reached through multicast loopback.
//   - The group listener uses SO_REUSEADDR to share port 1900 with the operating system's SSDP stack.
//   - Multicast leaves with a TTL of 2 (§1); a [Group] with a TTL of 0 keeps it on the host.
//   - Datagrams that aren't SSDP are dropped (§1.1.5) and logged with log/slog at debug level.
//
// # Control point
//
//   - [ControlPoint.Search] sends each M-SEARCH three times, 100 ms apart, and listens half a second
//     past MX for late answers (§1.3.2).
//   - [ControlPoint.SearchUnicast] waits the one second §1.3.2 suggests. Unicast search came with UDA
//     1.1, so UDA 1.0 devices like Gerbera and MiniDLNA ignore it.
//   - [ControlPoint.Watch] opens with a search and keeps one entry per device uuid, since one byebye
//     speaks for the whole device (§1.2.3).
//   - A changed BOOTID means a reboot and a new device; ssdp:update moves it forward instead (§1.2.4).
//   - A device expires when every announcement outlived its max-age; a missing max-age counts as 1800
//     seconds (§1.2.2).
//   - Refreshes that change nothing raise no event.
//
// # Device
//
//   - [NewDevice] waits a random 0 to 100 ms, then sends every announcement three times (§1.2.2).
//   - Announcements refresh at a random moment between a quarter and half of max-age (§1.2.2).
//   - Multicast searches are answered within MX, capped at 5 seconds, from a timer (§1.3.3).
//   - The last half second of MX stays out of that draw, since some control points, like
//     async-upnp-client, stop listening exactly at MX; §1.3.3 lets a device assume a shorter MX.
//   - Searches with another MAN or without MX get no answer, never an error response (§1.3.3).
//   - Close sends ssdp:byebye for every announcement (§1.2.3).
//
// [UDA 1.1]: https://upnp.org/specs/arch/UPnP-arch-DeviceArchitecture-v1.1.pdf
// [UDA 2.0]: https://upnp.org/specs/arch/UPnP-arch-DeviceArchitecture-v2.0.pdf
// [SSDP/1.0]: https://datatracker.ietf.org/doc/html/draft-cai-ssdp-v1-03
package ssdp
