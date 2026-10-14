package ssdp

import (
	"fmt"
	"strconv"
	"strings"
)

type TargetType int

const (
	TargetAll TargetType = iota + 1
	TargetRootDevice
	TargetUuid
	TargetDeviceType
	TargetServiceType
	TargetVendor
)

const (
	targetAll        = "ssdp:all"
	targetRootDevice = "upnp:rootdevice"
	uuidPrefix       = "uuid:"
	urnScheme        = "urn"
	urnPrefix        = urnScheme + urnSeparator
	urnSeparator     = ":"
	urnParts         = 5
	urnDevice        = "device"
	urnService       = "service"
	domainPeriod     = "."
	domainHyphen     = "-"

	standardDomain = "schemas-upnp-org"

	longestTypeName = 64
)

func (t TargetType) String() string {
	switch t {
	case TargetAll:
		return "all"
	case TargetRootDevice:
		return "root device"
	case TargetUuid:
		return "uuid"
	case TargetDeviceType:
		return "device type"
	case TargetServiceType:
		return "service type"
	case TargetVendor:
		return "vendor"
	default:
		return "unknown"
	}
}

// Target is what ST searches for and NT announces.
type Target struct {
	typ     TargetType
	value   string
	domain  string
	name    string
	version int
}

func newAllTarget() Target {
	return Target{
		typ:   TargetAll,
		value: targetAll,
	}
}

func newRootDeviceTarget() Target {
	return Target{
		typ:   TargetRootDevice,
		value: targetRootDevice,
	}
}

func newUuidTarget(uuid string) Target {
	return Target{
		typ:   TargetUuid,
		value: uuidPrefix + uuid,
		name:  uuid,
	}
}

func newDeviceTypeTarget(domain, name string, version int) (Target, error) {
	return newTypeTarget(TargetDeviceType, urnDevice, domain, name, version)
}

func newServiceTypeTarget(domain, name string, version int) (Target, error) {
	return newTypeTarget(TargetServiceType, urnService, domain, name, version)
}

// NewVendorTarget is a target outside the UDA grammar.
func newVendorTarget(value string) Target {
	return Target{
		typ:   TargetVendor,
		value: value,
	}
}

func newTypeTarget(typ TargetType, category, domain, name string, version int) (Target, error) {
	domain = strings.ReplaceAll(domain, domainPeriod, domainHyphen)

	if domain == "" || name == "" || len(name) > longestTypeName || version < 1 {
		return Target{}, fmt.Errorf("%w: %s %s:%s:%d is not a valid type", ErrMalformed, category, domain, name, version)
	}

	parts := []string{
		urnScheme,
		domain,
		category,
		name,
		strconv.Itoa(version),
	}

	return Target{
		typ:     typ,
		value:   strings.Join(parts, urnSeparator),
		domain:  domain,
		name:    name,
		version: version,
	}, nil
}

// ParseTarget reads an ST or NT value; a urn outside the device and service grammar becomes a vendor target.
func ParseTarget(value string) (Target, error) {
	switch {
	case value == "":
		return Target{}, fmt.Errorf("%w: empty target", ErrMalformed)
	case value == targetAll:
		return newAllTarget(), nil
	case value == targetRootDevice:
		return newRootDeviceTarget(), nil
	case strings.HasPrefix(value, uuidPrefix):
		uuid := strings.TrimPrefix(value, uuidPrefix)
		if uuid == "" {
			return Target{}, fmt.Errorf("%w: %q has no uuid", ErrMalformed, value)
		}

		return newUuidTarget(uuid), nil
	case strings.HasPrefix(value, urnPrefix):
		return parseUrnTarget(value), nil
	default:
		return newVendorTarget(value), nil
	}
}

func parseUrnTarget(value string) Target {
	parts := strings.Split(value, urnSeparator)
	if len(parts) != urnParts {
		return newVendorTarget(value)
	}

	version, err := strconv.Atoi(parts[4])
	if err != nil {
		return newVendorTarget(value)
	}

	var (
		target  Target
		typeErr error
	)

	switch parts[2] {
	case urnDevice:
		target, typeErr = newDeviceTypeTarget(parts[1], parts[3], version)
	case urnService:
		target, typeErr = newServiceTypeTarget(parts[1], parts[3], version)
	default:
		return newVendorTarget(value)
	}

	if typeErr != nil {
		return newVendorTarget(value)
	}

	return target
}

func (t Target) Type() TargetType {
	return t.typ
}

func (t Target) IsZero() bool {
	return t.typ == 0
}

func (t Target) Domain() string {
	return t.domain
}

// Name is the type name of device and service types, and the uuid of uuid targets.
func (t Target) Name() string {
	return t.name
}

func (t Target) Version() int {
	return t.version
}

func (t Target) String() string {
	return t.value
}

// Answers reports whether a device announcing t answers a search for search.
func (t Target) Answers(search Target) bool {
	switch search.typ {
	case TargetAll:
		return true
	case TargetDeviceType, TargetServiceType:
		return t.typ == search.typ && t.domain == search.domain && t.name == search.name && t.version >= search.version
	default:
		return t.typ == search.typ && t.value == search.value
	}
}

func (t Target) responseTarget(search Target) Target {
	if search.typ == TargetAll {
		return t
	}

	return search
}
