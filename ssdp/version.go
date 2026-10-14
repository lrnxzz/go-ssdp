package ssdp

import (
	"fmt"
	"strconv"
	"strings"
)

const versionSeparator = "."

var (
	Version10 = newVersion(1, 0)
	Version11 = newVersion(1, 1)
	Version20 = newVersion(2, 0)
)

// Version is a UPnP Device Architecture version, as SERVER and USER-AGENT declare it.
type Version struct {
	major int
	minor int
}

func newVersion(major, minor int) Version {
	return Version{
		major: major,
		minor: minor,
	}
}

func ParseVersion(value string) (Version, error) {
	majorText, minorText, ok := strings.Cut(value, versionSeparator)
	if !ok {
		return Version{}, fmt.Errorf("%w: version %q has no minor part", ErrMalformed, value)
	}

	major, majorErr := strconv.Atoi(majorText)
	minor, minorErr := strconv.Atoi(minorText)

	if majorErr != nil || minorErr != nil || major < 1 || minor < 0 {
		return Version{}, fmt.Errorf("%w: version %q is not major.minor", ErrMalformed, value)
	}

	return newVersion(major, minor), nil
}

func (v Version) Major() int {
	return v.major
}

func (v Version) Minor() int {
	return v.minor
}

func (v Version) IsZero() bool {
	return v.major == 0
}

func (v Version) AtLeast(other Version) bool {
	return v.major > other.major || (v.major == other.major && v.minor >= other.minor)
}

func (v Version) String() string {
	return strconv.Itoa(v.major) + versionSeparator + strconv.Itoa(v.minor)
}

func (v Version) rules() Version {
	if v.IsZero() {
		return Version10
	}

	return v
}
