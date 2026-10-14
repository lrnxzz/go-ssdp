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
	methodSearch = "M-SEARCH"
	methodNotify = "NOTIFY"

	requestTarget = "*"
	protocolName  = "HTTP/1.1"
	statusOk      = "200"
	reasonOk      = "OK"
)

const (
	headerHost                = "HOST"
	headerMan                 = "MAN"
	headerMaxWait             = "MX"
	headerSearchTarget        = "ST"
	headerNotificationType    = "NT"
	headerNotificationSubtype = "NTS"
	headerServiceName         = "USN"
	headerLocation            = "LOCATION"
	headerServer              = "SERVER"
	headerUserAgent           = "USER-AGENT"
	headerCacheControl        = "CACHE-CONTROL"
	headerDate                = "DATE"
	headerExt                 = "EXT"
	headerBootId              = "BOOTID.UPNP.ORG"
	headerConfigId            = "CONFIGID.UPNP.ORG"
	headerNextBootId          = "NEXTBOOTID.UPNP.ORG"
	headerSearchPort          = "SEARCHPORT.UPNP.ORG"
	headerSecureLocation      = "SECURELOCATION.UPNP.ORG"
	headerControlPointName    = "CPFN.UPNP.ORG"
	headerControlPointUuid    = "CPUUID.UPNP.ORG"
	headerTcpPort             = "TCPPORT.UPNP.ORG"
)

const (
	manDiscover = `"ssdp:discover"`

	subtypeAlive  = "ssdp:alive"
	subtypeByeBye = "ssdp:byebye"
	subtypeUpdate = "ssdp:update"

	maxAgeDirective = "max-age"
)

// The bounds the specification sets on MX, max-age, the advertised ports and the boot and
// configuration counters.
const (
	maxWaitFloor    = time.Second
	maxWaitCeiling  = 5 * time.Second
	maxAgeFloor     = 30 * time.Minute
	portFloor       = 49152
	portCeiling     = 65535
	counterCeiling  = 1<<31 - 1
	configIdCeiling = 1<<24 - 1
)

const (
	repetitions         = 3
	repetitionInterval  = 100 * time.Millisecond
	initialDelayCeiling = 100 * time.Millisecond
	searchGrace         = 500 * time.Millisecond
	unicastSearchWait   = time.Second
	responseMargin      = 500 * time.Millisecond
)

const defaultInterface = "default"

var (
	// ErrMalformed is a datagram that is not a well-formed SSDP message.
	ErrMalformed = errors.New("malformed ssdp message")

	// ErrUnexpected is a well-formed message of another type than the one expected.
	ErrUnexpected = errors.New("unexpected ssdp message")

	// ErrNonConformant is a message that breaks a MUST of the version it declares.
	ErrNonConformant = errors.New("ssdp message breaks the specification")
)
