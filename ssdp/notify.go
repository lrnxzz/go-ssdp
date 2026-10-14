package ssdp

import (
	"fmt"
	"net/http"
	"time"
)

// Alive announces that a device is available.
type Alive struct {
	Host           string
	Target         Target
	ServiceName    ServiceName
	Location       string
	Server         ProductTokens
	MaxAge         time.Duration
	BootId         Optional[int]
	ConfigId       Optional[int]
	SearchPort     Optional[int]
	SecureLocation string

	Extensions http.Header
}

// ByeBye cancels an announcement before it expires.
type ByeBye struct {
	Host        string
	Target      Target
	ServiceName ServiceName
	BootId      Optional[int]
	ConfigId    Optional[int]

	Extensions http.Header
}

// Update announces that a multi-homed device is about to change its BOOTID.
type Update struct {
	Host           string
	Target         Target
	ServiceName    ServiceName
	Location       string
	BootId         Optional[int]
	ConfigId       Optional[int]
	NextBootId     Optional[int]
	SearchPort     Optional[int]
	SecureLocation string

	Extensions http.Header
}

func (a Alive) Type() MessageType {
	return MessageAlive
}

func (a Alive) MarshalBinary() ([]byte, error) {
	if err := checkIdentity(a.Target, a.ServiceName); err != nil {
		return nil, err
	}

	return newWriter().
		requestLine(methodNotify).
		optional(headerHost, a.Host).
		maxAge(a.MaxAge).
		optional(headerLocation, a.Location).
		field(headerNotificationType, a.Target.String()).
		field(headerNotificationSubtype, subtypeAlive).
		optional(headerServer, a.Server.String()).
		field(headerServiceName, a.ServiceName.String()).
		number(headerBootId, a.BootId).
		number(headerConfigId, a.ConfigId).
		number(headerSearchPort, a.SearchPort).
		optional(headerSecureLocation, a.SecureLocation).
		extensions(a.Extensions).
		bytes(), nil
}

func (a *Alive) UnmarshalBinary(data []byte) error {
	return unmarshal(data, a)
}

func (a *Alive) read(f frame) error {
	target, serviceName, err := readIdentity(f)
	if err != nil {
		return err
	}

	location, err := f.url(headerLocation)
	if err != nil {
		return err
	}

	maxAge, err := f.maxAge()
	if err != nil {
		return err
	}

	bootId, configId, err := readCounters(f)
	if err != nil {
		return err
	}

	searchPort, err := f.port(headerSearchPort)
	if err != nil {
		return err
	}

	secureLocation, err := f.url(headerSecureLocation)
	if err != nil {
		return err
	}

	*a = Alive{
		Host:           f.header.Get(headerHost),
		Target:         target,
		ServiceName:    serviceName,
		Location:       location,
		Server:         f.productTokens(headerServer),
		MaxAge:         maxAge,
		BootId:         bootId,
		ConfigId:       configId,
		SearchPort:     searchPort,
		SecureLocation: secureLocation,

		Extensions: f.extensions(),
	}

	return nil
}

func (a Alive) Check() error {
	rules := a.Server.Upnp().rules()
	violations := checkNotification(a.Host, a.Target)

	if a.MaxAge <= 0 {
		violations = append(violations, violation("%s with max-age is required", headerCacheControl))
	}

	if a.Location == "" {
		violations = append(violations, violation("%s is required", headerLocation))
	}

	if a.Server.IsZero() {
		violations = append(violations, violation("%s is required", headerServer))
	}

	violations = append(violations,
		requiredSince(rules, Version11, a.BootId.IsPresent(), headerBootId),
		requiredSince(rules, Version11, a.ConfigId.IsPresent(), headerConfigId),
	)

	return violationsOf(violations)
}

func (b ByeBye) Type() MessageType {
	return MessageByeBye
}

func (b ByeBye) MarshalBinary() ([]byte, error) {
	if err := checkIdentity(b.Target, b.ServiceName); err != nil {
		return nil, err
	}

	return newWriter().
		requestLine(methodNotify).
		optional(headerHost, b.Host).
		field(headerNotificationType, b.Target.String()).
		field(headerNotificationSubtype, subtypeByeBye).
		field(headerServiceName, b.ServiceName.String()).
		number(headerBootId, b.BootId).
		number(headerConfigId, b.ConfigId).
		extensions(b.Extensions).
		bytes(), nil
}

func (b *ByeBye) UnmarshalBinary(data []byte) error {
	return unmarshal(data, b)
}

func (b *ByeBye) read(f frame) error {
	target, serviceName, err := readIdentity(f)
	if err != nil {
		return err
	}

	bootId, configId, err := readCounters(f)
	if err != nil {
		return err
	}

	*b = ByeBye{
		Host:        f.header.Get(headerHost),
		Target:      target,
		ServiceName: serviceName,
		BootId:      bootId,
		ConfigId:    configId,

		Extensions: f.extensions(),
	}

	return nil
}

func (b ByeBye) Check() error {
	return violationsOf(checkNotification(b.Host, b.Target))
}

func (u Update) Type() MessageType {
	return MessageUpdate
}

func (u Update) MarshalBinary() ([]byte, error) {
	if err := checkIdentity(u.Target, u.ServiceName); err != nil {
		return nil, err
	}

	return newWriter().
		requestLine(methodNotify).
		optional(headerHost, u.Host).
		optional(headerLocation, u.Location).
		field(headerNotificationType, u.Target.String()).
		field(headerNotificationSubtype, subtypeUpdate).
		field(headerServiceName, u.ServiceName.String()).
		number(headerBootId, u.BootId).
		number(headerConfigId, u.ConfigId).
		number(headerNextBootId, u.NextBootId).
		number(headerSearchPort, u.SearchPort).
		optional(headerSecureLocation, u.SecureLocation).
		extensions(u.Extensions).
		bytes(), nil
}

func (u *Update) UnmarshalBinary(data []byte) error {
	return unmarshal(data, u)
}

func (u *Update) read(f frame) error {
	target, serviceName, err := readIdentity(f)
	if err != nil {
		return err
	}

	location, err := f.url(headerLocation)
	if err != nil {
		return err
	}

	bootId, configId, err := readCounters(f)
	if err != nil {
		return err
	}

	nextBootId, err := f.counter(headerNextBootId)
	if err != nil {
		return err
	}

	searchPort, err := f.port(headerSearchPort)
	if err != nil {
		return err
	}

	secureLocation, err := f.url(headerSecureLocation)
	if err != nil {
		return err
	}

	*u = Update{
		Host:           f.header.Get(headerHost),
		Target:         target,
		ServiceName:    serviceName,
		Location:       location,
		BootId:         bootId,
		ConfigId:       configId,
		NextBootId:     nextBootId,
		SearchPort:     searchPort,
		SecureLocation: secureLocation,

		Extensions: f.extensions(),
	}

	return nil
}

func (u Update) Check() error {
	violations := checkNotification(u.Host, u.Target)

	if u.Location == "" {
		violations = append(violations, violation("%s is required", headerLocation))
	}

	violations = append(violations,
		requiredSince(Version11, Version11, u.BootId.IsPresent(), headerBootId),
		requiredSince(Version11, Version11, u.ConfigId.IsPresent(), headerConfigId),
		requiredSince(Version11, Version11, u.NextBootId.IsPresent(), headerNextBootId),
	)

	bootId, hasBootId := u.BootId.Get()
	nextBootId, hasNextBootId := u.NextBootId.Get()

	if hasBootId && hasNextBootId && nextBootId <= bootId {
		violations = append(violations, violation("%s %d is not greater than %s %d", headerNextBootId, nextBootId, headerBootId, bootId))
	}

	return violationsOf(violations)
}

func checkIdentity(target Target, serviceName ServiceName) error {
	if target.IsZero() || serviceName.IsZero() {
		return fmt.Errorf("%w: a notification needs NT and USN", ErrMalformed)
	}

	return nil
}

func readIdentity(f frame) (Target, ServiceName, error) {
	target, err := f.target(headerNotificationType)
	if err != nil {
		return Target{}, ServiceName{}, err
	}

	serviceName, err := f.serviceName()
	if err != nil {
		return Target{}, ServiceName{}, err
	}

	return target, serviceName, nil
}

func readCounters(f frame) (Optional[int], Optional[int], error) {
	bootId, err := f.counter(headerBootId)
	if err != nil {
		return Optional[int]{}, Optional[int]{}, err
	}

	configId, err := f.counter(headerConfigId)
	if err != nil {
		return Optional[int]{}, Optional[int]{}, err
	}

	return bootId, configId, nil
}

func checkNotification(host string, target Target) []error {
	violations := make([]error, 0)

	if host == "" {
		violations = append(violations, violation("%s is required", headerHost))
	}

	if target.typ == TargetAll {
		violations = append(violations, violation("%s cannot be %s", headerNotificationType, targetAll))
	}

	return violations
}
