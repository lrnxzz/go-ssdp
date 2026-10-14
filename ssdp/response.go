package ssdp

import (
	"fmt"
	"net/http"
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
		field(HeaderExt, "").
		optional(HeaderLocation, r.Location).
		optional(HeaderServer, r.Server.String()).
		field(HeaderSearchTarget, r.Target.String()).
		field(HeaderServiceName, r.ServiceName.String()).
		number(HeaderBootId, r.BootId).
		number(HeaderConfigId, r.ConfigId).
		number(HeaderSearchPort, r.SearchPort).
		optional(HeaderSecureLocation, r.SecureLocation).
		extensions(r.Extensions).
		bytes(), nil
}

func (r *SearchResponse) UnmarshalBinary(data []byte) error {
	return unmarshal(data, r)
}

func (r *SearchResponse) read(f frame) error {
	target, err := f.target(HeaderSearchTarget)
	if err != nil {
		return err
	}

	serviceName, err := f.serviceName()
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

	date, err := f.date()
	if err != nil {
		return err
	}

	bootId, err := f.counter(HeaderBootId)
	if err != nil {
		return err
	}

	configId, err := f.counter(HeaderConfigId)
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

	*r = SearchResponse{
		Target:         target,
		ServiceName:    serviceName,
		Location:       location,
		Server:         f.productTokens(HeaderServer),
		MaxAge:         maxAge,
		Date:           date,
		BootId:         bootId,
		ConfigId:       configId,
		SearchPort:     searchPort,
		SecureLocation: secureLocation,

		Extensions: f.extensions(),

		withoutExt: !f.has(HeaderExt),
	}

	return nil
}

func (r SearchResponse) Check() error {
	rules := r.Server.Upnp().rules()
	violations := make([]error, 0)

	if r.MaxAge <= 0 {
		violations = append(violations, violation("%s with max-age is required", HeaderCacheControl))
	}

	if r.withoutExt {
		violations = append(violations, violation("%s is required", HeaderExt))
	}

	if r.Location == "" {
		violations = append(violations, violation("%s is required", HeaderLocation))
	}

	if r.Server.IsZero() {
		violations = append(violations, violation("%s is required", HeaderServer))
	}

	violations = append(violations,
		requiredSince(rules, Version11, r.BootId.IsPresent(), HeaderBootId),
	)

	return violationsOf(violations)
}
