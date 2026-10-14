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
		requestLine(MethodNotify).
		optional(HeaderHost, a.Host).
		maxAge(a.MaxAge).
		optional(HeaderLocation, a.Location).
		field(HeaderNotificationType, a.Target.String()).
		field(HeaderNotificationSubtype, SubtypeAlive).
		optional(HeaderServer, a.Server.String()).
		field(HeaderServiceName, a.ServiceName.String()).
		number(HeaderBootId, a.BootId).
		number(HeaderConfigId, a.ConfigId).
		number(HeaderSearchPort, a.SearchPort).
		optional(HeaderSecureLocation, a.SecureLocation).
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

	location, err := f.url(HeaderLocation)
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

	searchPort, err := f.port(HeaderSearchPort)
	if err != nil {
		return err
	}

	secureLocation, err := f.url(HeaderSecureLocation)
	if err != nil {
		return err
	}

	*a = Alive{
		Host:           f.header.Get(HeaderHost),
		Target:         target,
		ServiceName:    serviceName,
		Location:       location,
		Server:         f.productTokens(HeaderServer),
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
		violations = append(violations, violation("%s with max-age is required", HeaderCacheControl))
	}

	if a.Location == "" {
		violations = append(violations, violation("%s is required", HeaderLocation))
	}

	if a.Server.IsZero() {
		violations = append(violations, violation("%s is required", HeaderServer))
	}

	violations = append(violations,
		requiredSince(rules, Version11, a.BootId.IsPresent(), HeaderBootId),
		requiredSince(rules, Version11, a.ConfigId.IsPresent(), HeaderConfigId),
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
		requestLine(MethodNotify).
		optional(HeaderHost, b.Host).
		field(HeaderNotificationType, b.Target.String()).
		field(HeaderNotificationSubtype, SubtypeByeBye).
		field(HeaderServiceName, b.ServiceName.String()).
		number(HeaderBootId, b.BootId).
		number(HeaderConfigId, b.ConfigId).
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
		Host:        f.header.Get(HeaderHost),
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
		requestLine(MethodNotify).
		optional(HeaderHost, u.Host).
		optional(HeaderLocation, u.Location).
		field(HeaderNotificationType, u.Target.String()).
		field(HeaderNotificationSubtype, SubtypeUpdate).
		field(HeaderServiceName, u.ServiceName.String()).
		number(HeaderBootId, u.BootId).
		number(HeaderConfigId, u.ConfigId).
		number(HeaderNextBootId, u.NextBootId).
		number(HeaderSearchPort, u.SearchPort).
		optional(HeaderSecureLocation, u.SecureLocation).
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

	location, err := f.url(HeaderLocation)
	if err != nil {
		return err
	}

	bootId, configId, err := readCounters(f)
	if err != nil {
		return err
	}

	nextBootId, err := f.counter(HeaderNextBootId)
	if err != nil {
		return err
	}

	searchPort, err := f.port(HeaderSearchPort)
	if err != nil {
		return err
	}

	secureLocation, err := f.url(HeaderSecureLocation)
	if err != nil {
		return err
	}

	*u = Update{
		Host:           f.header.Get(HeaderHost),
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
		violations = append(violations, violation("%s is required", HeaderLocation))
	}

	violations = append(violations,
		requiredSince(Version11, Version11, u.BootId.IsPresent(), HeaderBootId),
		requiredSince(Version11, Version11, u.ConfigId.IsPresent(), HeaderConfigId),
		requiredSince(Version11, Version11, u.NextBootId.IsPresent(), HeaderNextBootId),
	)

	bootId, hasBootId := u.BootId.Get()
	nextBootId, hasNextBootId := u.NextBootId.Get()

	if hasBootId && hasNextBootId && nextBootId <= bootId {
		violations = append(violations, violation("%s %d is not greater than %s %d", HeaderNextBootId, nextBootId, HeaderBootId, bootId))
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
	target, err := f.target(HeaderNotificationType)
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
	bootId, err := f.counter(HeaderBootId)
	if err != nil {
		return Optional[int]{}, Optional[int]{}, err
	}

	configId, err := f.counter(HeaderConfigId)
	if err != nil {
		return Optional[int]{}, Optional[int]{}, err
	}

	return bootId, configId, nil
}

func checkNotification(host string, target Target) []error {
	violations := make([]error, 0)

	if host == "" {
		violations = append(violations, violation("%s is required", HeaderHost))
	}

	if target.typ == TargetAll {
		violations = append(violations, violation("%s cannot be %s", HeaderNotificationType, targetAll))
	}

	return violations
}
