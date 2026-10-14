package ssdp

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const udaSearch = "M-SEARCH * HTTP/1.1\r\n" +
	"HOST: 239.255.255.250:1900\r\n" +
	"MAN: \"ssdp:discover\"\r\n" +
	"MX: 2\r\n" +
	"ST: urn:schemas-upnp-org:device:MediaRenderer:1\r\n" +
	"USER-AGENT: unix/5.1 UPnP/1.1 MyProduct/1.0\r\n" +
	"\r\n"

const udaUnicastSearch = "M-SEARCH * HTTP/1.1\r\n" +
	"HOST: 192.0.2.20:1900\r\n" +
	"MAN: \"ssdp:discover\"\r\n" +
	"ST: upnp:rootdevice\r\n" +
	"USER-AGENT: unix/5.1 UPnP/1.1 MyProduct/1.0\r\n" +
	"\r\n"

const uda20Search = "M-SEARCH * HTTP/1.1\r\n" +
	"HOST: 239.255.255.250:1900\r\n" +
	"MAN: \"ssdp:discover\"\r\n" +
	"MX: 1\r\n" +
	"ST: urn:schemas-upnp-org:device:MediaRenderer:1\r\n" +
	"USER-AGENT: unix/5.1 UPnP/2.0 MyProduct/1.0\r\n" +
	"CPFN.UPNP.ORG: Living Room Remote\r\n" +
	"CPUUID.UPNP.ORG: uuid:3fac1234-31f8-11b4-a222-08002b34c003\r\n" +
	"TCPPORT.UPNP.ORG: 49154\r\n" +
	"\r\n"

func Test_Search_MarshalBinary_WritesTheUdaLayout(t *testing.T) {
	unicast := Search{
		Host:      "192.0.2.20:1900",
		Target:    NewRootDeviceTarget(),
		UserAgent: mustProductTokens(t, exampleServer),
	}

	uda20 := Search{
		Host:      MulticastAddress,
		Target:    mustTarget(t, exampleDevice),
		MaxWait:   time.Second,
		UserAgent: mustProductTokens(t, "unix/5.1 UPnP/2.0 MyProduct/1.0"),

		ControlPointName: "Living Room Remote",
		ControlPointUuid: "uuid:3fac1234-31f8-11b4-a222-08002b34c003",
		TcpPort:          NewOptional(49154),
	}

	tests := map[string]Search{
		udaSearch:        exampleSearch(t),
		udaUnicastSearch: unicast,
		uda20Search:      uda20,
	}

	for expected, search := range tests {
		if got := string(marshal(t, search)); got != expected {
			t.Errorf("Expected\n%q\ngot\n%q", expected, got)
		}
	}
}

func Test_Search_UnmarshalBinary_ReadsEveryField(t *testing.T) {
	var search Search

	if err := search.UnmarshalBinary([]byte(uda20Search)); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if search.Host != MulticastAddress || search.Target.String() != exampleDevice || search.MaxWait != time.Second {
		t.Errorf("Expected a 1 second search for %s on %s, got %+v", exampleDevice, MulticastAddress, search)
	}

	if search.UserAgent.Upnp() != Version20 || search.ControlPointName != "Living Room Remote" {
		t.Errorf("Expected a named UDA 2.0 control point, got %+v", search)
	}

	if port, _ := search.TcpPort.Get(); port != 49154 || search.ControlPointUuid != "uuid:3fac1234-31f8-11b4-a222-08002b34c003" {
		t.Errorf("Expected TCP port 49154 and the control point uuid, got %+v", search)
	}

	expectConformant(t, search)
}

func Test_Search_RoundTrip(t *testing.T) {
	searches := []string{
		udaSearch,
		udaUnicastSearch,
		uda20Search,
	}

	for _, packet := range searches {
		if got := string(marshal(t, mustDecode[Search](t, []byte(packet)))); got != packet {
			t.Errorf("Expected\n%q\ngot\n%q", packet, got)
		}
	}
}

func Test_Search_MarshalBinary_RefusesWhatCannotBeEncoded(t *testing.T) {
	tests := map[string]Search{
		"no host": {
			Target:  NewAllTarget(),
			MaxWait: time.Second,
		},
		"no target": {
			Host:    MulticastAddress,
			MaxWait: time.Second,
		},
		"fraction of a second": {
			Host:    MulticastAddress,
			Target:  NewAllTarget(),
			MaxWait: 1500 * time.Millisecond,
		},
		"negative wait": {
			Host:    MulticastAddress,
			Target:  NewAllTarget(),
			MaxWait: -time.Second,
		},
	}

	for name, search := range tests {
		if _, err := search.MarshalBinary(); !errors.Is(err, ErrMalformed) {
			t.Errorf("Expected %s to be refused, got %v", name, err)
		}
	}
}

func Test_Search_UnmarshalBinary_RejectsAnotherMan(t *testing.T) {
	packets := []string{
		strings.Replace(udaSearch, `"ssdp:discover"`, "ssdp:discover", 1),
		strings.Replace(udaSearch, `"ssdp:discover"`, `"ssdp:other"`, 1),
		strings.Replace(udaSearch, "MAN: \"ssdp:discover\"\r\n", "", 1),
	}

	for _, packet := range packets {
		if _, err := Decode([]byte(packet)); !errors.Is(err, ErrMalformed) {
			t.Errorf("Expected %q to be malformed, got %v", packet, err)
		}
	}
}

func Test_Search_UnmarshalBinary_RejectsBrokenFields(t *testing.T) {
	tests := map[string]string{
		"words for MX":      strings.Replace(udaSearch, "MX: 2", "MX: soon", 1),
		"negative MX":       strings.Replace(udaSearch, "MX: 2", "MX: -1", 1),
		"no search target":  strings.Replace(udaSearch, "ST: "+exampleDevice+"\r\n", "", 1),
		"tcp port too low":  strings.Replace(uda20Search, "TCPPORT.UPNP.ORG: 49154", "TCPPORT.UPNP.ORG: 80", 1),
		"tcp port too high": strings.Replace(uda20Search, "TCPPORT.UPNP.ORG: 49154", "TCPPORT.UPNP.ORG: 65536", 1),
	}

	for name, packet := range tests {
		if _, err := Decode([]byte(packet)); !errors.Is(err, ErrMalformed) {
			t.Errorf("Expected %s to be malformed, got %v", name, err)
		}
	}
}

func Test_Search_IsMulticast(t *testing.T) {
	tests := map[string]bool{
		MulticastAddress:        true,
		"239.255.255.250":       true,
		"239.255.255.250:54321": true,
		"[ff02::c]:1900":        true,
		"192.0.2.20:1900":       false,
		"192.0.2.20":            false,
		"tv.example.com:1900":   false,
		"":                      false,
		"not an address at all": false,
	}

	for host, expected := range tests {
		search := Search{
			Host: host,
		}

		if got := search.IsMulticast(); got != expected {
			t.Errorf("Expected IsMulticast() for %q = %t, got %t", host, expected, got)
		}
	}
}

func Test_Search_Check(t *testing.T) {
	conformant := []string{
		udaSearch,
		udaUnicastSearch,
		uda20Search,
	}

	for _, packet := range conformant {
		expectConformant(t, mustDecode[Search](t, []byte(packet)))
	}

	withoutMaxWait := exampleSearch(t)
	withoutMaxWait.MaxWait = 0
	expectViolations(t, withoutMaxWait, 1)

	withoutName := mustDecode[Search](t, []byte(uda20Search))
	withoutName.ControlPointName = ""
	expectViolations(t, withoutName, 1)

	expectViolations(t, Search{}, 2)
}

func Test_Search_Type(t *testing.T) {
	if got := (Search{}).Type(); got != MessageSearch {
		t.Errorf("Expected %s, got %s", MessageSearch, got)
	}
}
