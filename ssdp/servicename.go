package ssdp

import (
	"fmt"
	"strings"
)

const serviceNameSeparator = "::"

// ServiceName is a USN: "uuid:<uuid>" alone, or "uuid:<uuid>::<target>".
type ServiceName struct {
	uuid   string
	target Target
}

func newServiceName(uuid string, target Target) ServiceName {
	if target.typ == TargetUuid {
		target = Target{}
	}

	return ServiceName{
		uuid:   uuid,
		target: target,
	}
}

// ParseServiceName reads a USN, accepting a uuid in any format.
func ParseServiceName(value string) (ServiceName, error) {
	rest, ok := strings.CutPrefix(value, uuidPrefix)
	if !ok || rest == "" {
		return ServiceName{}, fmt.Errorf("%w: service name %q doesn't start with %s", ErrMalformed, value, uuidPrefix)
	}

	uuid, targetText, hasTarget := strings.Cut(rest, serviceNameSeparator)
	if uuid == "" {
		return ServiceName{}, fmt.Errorf("%w: service name %q has no uuid", ErrMalformed, value)
	}

	if !hasTarget {
		return newServiceName(uuid, Target{}), nil
	}

	target, err := ParseTarget(targetText)
	if err != nil {
		return ServiceName{}, err
	}

	return newServiceName(uuid, target), nil
}

func (n ServiceName) Uuid() string {
	return n.uuid
}

// Target reports false for a bare uuid.
func (n ServiceName) Target() (Target, bool) {
	return n.target, !n.target.IsZero()
}

func (n ServiceName) IsZero() bool {
	return n.uuid == ""
}

func (n ServiceName) String() string {
	if n.target.IsZero() {
		return uuidPrefix + n.uuid
	}

	return uuidPrefix + n.uuid + serviceNameSeparator + n.target.String()
}
