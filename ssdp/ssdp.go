package ssdp

import (
	"errors"
	"time"
)

const (
	MulticastAddress = "239.255.255.250:1900"
	DefaultTtl       = 2
	HostOnlyTtl      = 0

	udpNetwork      = "udp4"
	maxDatagramSize = 8192
)

const (
	MethodSearch = "M-SEARCH"
	MethodNotify = "NOTIFY"

	requestTarget = "*"
	protocolName  = "HTTP/1.1"
	statusOk      = "200"
	reasonOk      = "OK"
)

const (
	HeaderHost                = "HOST"
	HeaderMan                 = "MAN"
	HeaderMaxWait             = "MX"
	HeaderSearchTarget        = "ST"
	HeaderNotificationType    = "NT"
	HeaderNotificationSubtype = "NTS"
	HeaderServiceName         = "USN"
	HeaderLocation            = "LOCATION"
	HeaderServer              = "SERVER"
	HeaderUserAgent           = "USER-AGENT"
	HeaderCacheControl        = "CACHE-CONTROL"
	HeaderDate                = "DATE"
	HeaderExt                 = "EXT"
	HeaderBootId              = "BOOTID.UPNP.ORG"
	HeaderConfigId            = "CONFIGID.UPNP.ORG"
	HeaderNextBootId          = "NEXTBOOTID.UPNP.ORG"
	HeaderSearchPort          = "SEARCHPORT.UPNP.ORG"
	HeaderSecureLocation      = "SECURELOCATION.UPNP.ORG"
	HeaderControlPointName    = "CPFN.UPNP.ORG"
	HeaderControlPointUuid    = "CPUUID.UPNP.ORG"
	HeaderTcpPort             = "TCPPORT.UPNP.ORG"
)

const (
	ManDiscover = `"ssdp:discover"`

	SubtypeAlive  = "ssdp:alive"
	SubtypeByeBye = "ssdp:byebye"
	SubtypeUpdate = "ssdp:update"

	maxAgeDirective = "max-age"
)

// The bounds the specification sets on MX, max-age, the advertised ports and the boot and
// configuration counters.
const (
	MaxWaitFloor    = time.Second
	MaxWaitCeiling  = 5 * time.Second
	MaxAgeFloor     = 30 * time.Minute
	PortFloor       = 49152
	PortCeiling     = 65535
	CounterCeiling  = 1<<31 - 1
	ConfigIdCeiling = 1<<24 - 1
)

const (
	repetitions         = 3
	repetitionInterval  = 100 * time.Millisecond
	initialDelayCeiling = 100 * time.Millisecond
	searchGrace         = 500 * time.Millisecond
	unicastSearchWait   = time.Second
	responseMargin      = 500 * time.Millisecond
)

const (
	logDropped       = "ssdp: datagram dropped"
	logIgnored       = "ssdp: message ignored"
	logSkipped       = "ssdp: interface skipped"
	logNotSent       = "ssdp: datagram not sent"
	logKeyFrom       = "from"
	logKeyInterface  = "interface"
	logKeyType       = "type"
	logKeyReason     = "reason"
	defaultInterface = "default"
	listenerName     = "group"
	allInterfaces    = "0.0.0.0"
)

var (
	// ErrMalformed is a datagram that is not a well-formed SSDP message.
	ErrMalformed = errors.New("malformed ssdp message")

	// ErrUnexpected is a well-formed message of another type than the one expected.
	ErrUnexpected = errors.New("unexpected ssdp message")

	// ErrNonConformant is a message that breaks a MUST of the version it declares.
	ErrNonConformant = errors.New("ssdp message breaks the specification")
)
