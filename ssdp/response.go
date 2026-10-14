package ssdp

import (
	"fmt"
	"net/http"
	"net/netip"
	"time"
)

// SearchResponse answers an M-SEARCH.
type SearchResponse struct {
	Target         Target
	ServiceName    ServiceName
	Location       string
	Server         ProductTokens
	MaxAge         time.Duration
	Date           time.Time
	BootId         Optional[int]
	ConfigId       Optional[int]
	SearchPort     Optional[int]
	SecureLocation string

	Extensions http.Header

	// From and Interface are where a control point received the answer; they don't travel in it.
	From      netip.AddrPort
	Interface string

	withoutExt bool
}

func (r SearchResponse) Type() MessageType {
	return MessageSearchResponse
}

func (r SearchResponse) MarshalBinary() ([]byte, error) {
	if r.Target.IsZero() || r.ServiceName.IsZero() {
		return nil, fmt.Errorf("%w: a search response needs ST and USN", ErrMalformed)
	}

	return newWriter().
		statusLine().
		maxAge(r.MaxAge).
		date(r.Date).
		field(headerExt, "").
		optional(headerLocation, r.Location).
		optional(headerServer, r.Server.String()).
		field(headerSearchTarget, r.Target.String()).
		field(headerServiceName, r.ServiceName.String()).
		number(headerBootId, r.BootId).
		number(headerConfigId, r.ConfigId).
		number(headerSearchPort, r.SearchPort).
		optional(headerSecureLocation, r.SecureLocation).
		extensions(r.Extensions).
		bytes(), nil
}

func (r *SearchResponse) UnmarshalBinary(data []byte) error {
	return unmarshal(data, r)
}

func (r *SearchResponse) read(f frame) error {
	target, err := f.target(headerSearchTarget)
	if err != nil {
		return err
	}

	serviceName, err := f.serviceName()
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

	date, err := f.date()
	if err != nil {
		return err
	}

	bootId, err := f.counter(headerBootId)
	if err != nil {
		return err
	}

	configId, err := f.counter(headerConfigId)
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

	*r = SearchResponse{
		Target:         target,
		ServiceName:    serviceName,
		Location:       location,
		Server:         f.productTokens(headerServer),
		MaxAge:         maxAge,
		Date:           date,
		BootId:         bootId,
		ConfigId:       configId,
		SearchPort:     searchPort,
		SecureLocation: secureLocation,

		Extensions: f.extensions(),

		withoutExt: !f.has(headerExt),
	}

	return nil
}

func (r SearchResponse) Check() error {
	rules := r.Server.Upnp().rules()
	violations := make([]error, 0)

	if r.MaxAge <= 0 {
		violations = append(violations, violation("%s with max-age is required", headerCacheControl))
	}

	if r.withoutExt {
		violations = append(violations, violation("%s is required", headerExt))
	}

	if r.Location == "" {
		violations = append(violations, violation("%s is required", headerLocation))
	}

	if r.Server.IsZero() {
		violations = append(violations, violation("%s is required", headerServer))
	}

	violations = append(violations,
		requiredSince(rules, Version11, r.BootId.IsPresent(), headerBootId),
	)

	return violationsOf(violations)
}
