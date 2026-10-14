package ssdp

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

const udaSearchResponse = "HTTP/1.1 200 OK\r\n" +
	"CACHE-CONTROL: max-age=1800\r\n" +
	"DATE: Tue, 13 Oct 2026 12:00:00 GMT\r\n" +
	"EXT: \r\n" +
	"LOCATION: http://192.0.2.20:49152/description.xml\r\n" +
	"SERVER: unix/5.1 UPnP/1.1 MyProduct/1.0\r\n" +
	"ST: urn:schemas-upnp-org:device:MediaRenderer:1\r\n" +
	"USN: uuid:2fac1234-31f8-11b4-a222-08002b34c003::urn:schemas-upnp-org:device:MediaRenderer:1\r\n" +
	"BOOTID.UPNP.ORG: 1\r\n" +
	"CONFIGID.UPNP.ORG: 7\r\n" +
	"SEARCHPORT.UPNP.ORG: 49153\r\n" +
	"\r\n"

func Test_SearchResponse_MarshalBinary_WritesTheUdaLayout(t *testing.T) {
	if got := string(marshal(t, exampleResponse(t))); got != udaSearchResponse {
		t.Errorf("Expected\n%q\ngot\n%q", udaSearchResponse, got)
	}
}

func Test_SearchResponse_UnmarshalBinary_ReadsEveryField(t *testing.T) {
	var response SearchResponse

	if err := response.UnmarshalBinary([]byte(udaSearchResponse)); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if !reflect.DeepEqual(response, exampleResponse(t)) {
		t.Errorf("Expected\n%+v\ngot\n%+v", exampleResponse(t), response)
	}

	expectConformant(t, response)
}

func Test_SearchResponse_RoundTrip(t *testing.T) {
	if got := string(marshal(t, mustDecode[SearchResponse](t, []byte(udaSearchResponse)))); got != udaSearchResponse {
		t.Errorf("Expected\n%q\ngot\n%q", udaSearchResponse, got)
	}
}

func Test_SearchResponse_ReadsTheSpacedMaxAge(t *testing.T) {
	packet := strings.Replace(udaSearchResponse, "max-age=1800", "max-age = 1800", 1)

	if got := mustDecode[SearchResponse](t, []byte(packet)).MaxAge; got != 30*time.Minute {
		t.Errorf("Expected 30 minutes, got %s", got)
	}
}

func Test_SearchResponse_IgnoresOtherCacheDirectives(t *testing.T) {
	packet := strings.Replace(udaSearchResponse, "max-age=1800", "no-cache, MAX-AGE = 60", 1)

	if got := mustDecode[SearchResponse](t, []byte(packet)).MaxAge; got != time.Minute {
		t.Errorf("Expected a minute, got %s", got)
	}
}

func Test_SearchResponse_IgnoresLeadingZeros(t *testing.T) {
	packet := strings.Replace(udaSearchResponse, "BOOTID.UPNP.ORG: 1", "BOOTID.UPNP.ORG: 0001", 1)

	if bootId, _ := mustDecode[SearchResponse](t, []byte(packet)).BootId.Get(); bootId != 1 {
		t.Errorf("Expected boot id 1, got %d", bootId)
	}
}

func Test_SearchResponse_Check_FollowsTheDeclaredVersion(t *testing.T) {
	withoutConfigId := exampleResponse(t)
	withoutConfigId.ConfigId = Optional[int]{}
	expectConformant(t, withoutConfigId)

	withoutBootId := exampleResponse(t)
	withoutBootId.BootId = Optional[int]{}
	expectViolations(t, withoutBootId, 1)

	withoutBootId.Server = mustProductTokens(t, "unix/5.1 UPnP/1.0 MyProduct/1.0")
	expectConformant(t, withoutBootId)
}

func Test_SearchResponse_MarshalBinary_AlwaysCarriesExt(t *testing.T) {
	response := SearchResponse{
		Target:      NewRootDeviceTarget(),
		ServiceName: NewServiceName(exampleUuid, NewRootDeviceTarget()),
	}

	if data := marshal(t, response); !bytes.Contains(data, []byte("\r\nEXT: \r\n")) {
		t.Errorf("Expected an empty EXT field, got %q", data)
	}
}

func Test_SearchResponse_Check_NoticesAMissingExt(t *testing.T) {
	packet := strings.Replace(udaSearchResponse, "EXT: \r\n", "", 1)

	expectViolations(t, mustDecode[SearchResponse](t, []byte(packet)), 1)
}

func Test_SearchResponse_Check_ReportsEveryMissingField(t *testing.T) {
	response := SearchResponse{
		Target:      NewRootDeviceTarget(),
		ServiceName: NewServiceName(exampleUuid, NewRootDeviceTarget()),
	}

	expectViolations(t, response, 3)
}

func Test_SearchResponse_UnmarshalBinary_RejectsBrokenFields(t *testing.T) {
	tests := map[string]string{
		"negative boot id":       strings.Replace(udaSearchResponse, "BOOTID.UPNP.ORG: 1", "BOOTID.UPNP.ORG: -1", 1),
		"boot id over 31 bits":   strings.Replace(udaSearchResponse, "BOOTID.UPNP.ORG: 1", "BOOTID.UPNP.ORG: 2147483648", 1),
		"search port too low":    strings.Replace(udaSearchResponse, "SEARCHPORT.UPNP.ORG: 49153", "SEARCHPORT.UPNP.ORG: 1900", 1),
		"relative location":      strings.Replace(udaSearchResponse, exampleLocation, "/description.xml", 1),
		"date that isn't a date": strings.Replace(udaSearchResponse, "Tue, 13 Oct 2026 12:00:00 GMT", "yesterday", 1),
		"max-age that isn't one": strings.Replace(udaSearchResponse, "max-age=1800", "max-age=soon", 1),
		"usn without uuid":       strings.Replace(udaSearchResponse, "USN: uuid:", "USN: ", 1),
		"no search target":       strings.Replace(udaSearchResponse, "ST: "+exampleDevice+"\r\n", "", 1),
		"plain secure location":  strings.Replace(udaSearchResponse, "EXT: \r\n", "EXT: \r\nSECURELOCATION.UPNP.ORG: nowhere\r\n", 1),
	}

	for name, packet := range tests {
		if _, err := Decode([]byte(packet)); !errors.Is(err, ErrMalformed) {
			t.Errorf("Expected %s to be malformed, got %v", name, err)
		}
	}
}

func Test_SearchResponse_MarshalBinary_RefusesMissingIdentity(t *testing.T) {
	tests := map[string]SearchResponse{
		"no target": {
			ServiceName: NewServiceName(exampleUuid, NewRootDeviceTarget()),
		},
		"no service name": {
			Target: NewRootDeviceTarget(),
		},
	}

	for name, response := range tests {
		if _, err := response.MarshalBinary(); !errors.Is(err, ErrMalformed) {
			t.Errorf("Expected %s to be refused, got %v", name, err)
		}
	}
}

func Test_SearchResponse_Type(t *testing.T) {
	if got := (SearchResponse{}).Type(); got != MessageSearchResponse {
		t.Errorf("Expected %s, got %s", MessageSearchResponse, got)
	}
}
