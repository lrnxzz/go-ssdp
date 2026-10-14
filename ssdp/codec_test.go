package ssdp

import (
	"bytes"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

const foldedSearch = "M-SEARCH * HTTP/1.1\nHOST:0\nMAN:\"ssdp:discover\"\nST:0\n "

func Test_Decode_TellsTheMessagesApart(t *testing.T) {
	for _, example := range examples(t) {
		message, err := Decode(marshal(t, example))
		if err != nil {
			t.Errorf("Failed to decode a %s: %v", example.Type(), err)

			continue
		}

		if message.Type() != example.Type() {
			t.Errorf("Expected a %s, got %s", example.Type(), message.Type())
		}
	}
}

func Test_Decode_ReadsBackWhatTheCodecWrites(t *testing.T) {
	for _, example := range examples(t) {
		message, err := Decode(marshal(t, example))
		if err != nil {
			t.Fatalf("Failed to decode a %s: %v", example.Type(), err)
		}

		if !reflect.DeepEqual(message, example) {
			t.Errorf("Expected\n%+v\ngot\n%+v", example, message)
		}
	}
}

func Test_Decode_RejectsOtherStartLines(t *testing.T) {
	packets := []string{
		"",
		"GET / HTTP/1.1\r\n\r\n",
		"HTTP/1.1 404 Not Found\r\n\r\n",
		"M-SEARCH /path HTTP/1.1\r\nMAN: \"ssdp:discover\"\r\nST: ssdp:all\r\n\r\n",
		"NOTIFY * HTTP/1.0\r\n\r\n",
		"SUBSCRIBE * HTTP/1.1\r\n\r\n",
		"\x00\x01\x02",
	}

	for _, packet := range packets {
		if _, err := Decode([]byte(packet)); !errors.Is(err, ErrUnexpected) {
			t.Errorf("Expected %q to be unexpected, got %v", packet, err)
		}
	}
}

func Test_Decode_RejectsBrokenFieldLines(t *testing.T) {
	packets := []string{
		"NOTIFY * HTTP/1.1\r\nno colon here\r\n\r\n",
		"HTTP/1.1 200 OK\r\n: no name\r\n\r\n",
	}

	for _, packet := range packets {
		if _, err := Decode([]byte(packet)); !errors.Is(err, ErrMalformed) {
			t.Errorf("Expected %q to be malformed, got %v", packet, err)
		}
	}
}

func Test_Decode_RejectsUnknownNotificationSubtypes(t *testing.T) {
	packet := strings.Replace(udaAlive, subtypeAlive, "ssdp:propchange", 1)

	if _, err := Decode([]byte(packet)); !errors.Is(err, ErrMalformed) {
		t.Errorf("Expected an unknown NTS to be malformed, got %v", err)
	}
}

func Test_Decode_AcceptsADatagramWithoutTheFinalBlankLine(t *testing.T) {
	packet := strings.TrimSuffix(udaSearchResponse, "\r\n")

	if got := mustDecode[SearchResponse](t, []byte(packet)); !reflect.DeepEqual(got, exampleResponse(t)) {
		t.Errorf("Expected the example response, got %+v", got)
	}
}

func Test_Decode_AcceptsBareLineFeeds(t *testing.T) {
	packet := strings.ReplaceAll(udaByeBye, "\r\n", "\n")

	if got := mustDecode[ByeBye](t, []byte(packet)); !reflect.DeepEqual(got, exampleByeBye(t)) {
		t.Errorf("Expected the example byebye, got %+v", got)
	}
}

func Test_Decode_TrimsFieldValues(t *testing.T) {
	if got := mustDecode[SearchRequest](t, []byte(foldedSearch)).Target.String(); got != "0" {
		t.Errorf("Expected ST %q, got %q", "0", got)
	}

	padded := strings.Replace(udaSearch, "ST: ", "ST:   ", 1)
	if got := mustDecode[SearchRequest](t, []byte(padded)).Target.String(); got != exampleDevice {
		t.Errorf("Expected ST %q, got %q", exampleDevice, got)
	}
}

func Test_Decode_ReadsFieldNamesInAnyCase(t *testing.T) {
	lines := strings.Split(udaSearchResponse, "\r\n")

	for index, line := range lines[1:] {
		name, value, ok := strings.Cut(line, fieldSeparator)
		if ok {
			lines[index+1] = strings.ToLower(name) + fieldSeparator + value
		}
	}

	packet := strings.Join(lines, "\r\n")

	if got := mustDecode[SearchResponse](t, []byte(packet)); !reflect.DeepEqual(got, exampleResponse(t)) {
		t.Errorf("Expected the lower-case fields to be read, got %+v", got)
	}
}

func Test_Decode_KeepsVendorFieldsAsExtensions(t *testing.T) {
	alive := exampleAlive(t)
	alive.Extensions = http.Header{
		"Myheader.philips.com": []string{
			"some value",
		},
		"Myheader.sony.com": []string{
			"other value",
		},
	}

	decoded := mustDecode[Alive](t, marshal(t, alive))

	if !reflect.DeepEqual(decoded.Extensions, alive.Extensions) {
		t.Errorf("Expected extensions %v, got %v", alive.Extensions, decoded.Extensions)
	}
}

func Test_Decode_LeavesExtensionsNilWithoutVendorFields(t *testing.T) {
	if extensions := mustDecode[Alive](t, []byte(udaAlive)).Extensions; extensions != nil {
		t.Errorf("Expected no extensions, got %v", extensions)
	}
}

func Test_Decode_CopiesWhatItReads(t *testing.T) {
	buffer := []byte(udaSearchResponse)
	response := mustDecode[SearchResponse](t, buffer)

	for index := range buffer {
		buffer[index] = 'x'
	}

	if !reflect.DeepEqual(response, exampleResponse(t)) {
		t.Errorf("Expected the decoded response to survive the buffer being reused, got %+v", response)
	}
}

func Test_Unmarshal_RefusesAnotherMessageType(t *testing.T) {
	var alive Alive

	if err := alive.UnmarshalBinary([]byte(udaSearch)); !errors.Is(err, ErrUnexpected) {
		t.Errorf("Expected a search to be unexpected where an alive is, got %v", err)
	}
}

func Test_Unmarshal_ReportsMalformedData(t *testing.T) {
	var response SearchResponse

	if err := response.UnmarshalBinary([]byte("HTTP/1.1 200 OK\r\nbroken\r\n\r\n")); !errors.Is(err, ErrMalformed) {
		t.Errorf("Expected a malformed response, got %v", err)
	}
}

func Test_Writer_SortsExtensions(t *testing.T) {
	alive := exampleAlive(t)
	alive.Extensions = http.Header{
		"Zulu.example.com": []string{
			"last",
		},
		"Alpha.example.com": []string{
			"first",
		},
	}

	first := marshal(t, alive)

	if !bytes.Equal(first, marshal(t, alive)) {
		t.Error("Expected the same message to encode to the same bytes")
	}

	if bytes.Index(first, []byte("Alpha.example.com")) > bytes.Index(first, []byte("Zulu.example.com")) {
		t.Errorf("Expected extensions in name order, got %q", first)
	}
}

func Test_Violations(t *testing.T) {
	violations := Violations{
		violation("%s is required", headerHost),
		violation("%s is required", headerLocation),
	}

	if !errors.Is(violations, ErrNonConformant) {
		t.Error("Expected violations to unwrap to ErrNonConformant")
	}

	expected := "ssdp message breaks the specification: HOST is required; ssdp message breaks the specification: LOCATION is required"
	if got := violations.Error(); got != expected {
		t.Errorf("Expected %q, got %q", expected, got)
	}
}

func Test_ViolationsOf_ReturnsNilWhenNothingBroke(t *testing.T) {
	nothing := []error{
		nil,
		nil,
	}

	if err := violationsOf(nothing); err != nil {
		t.Errorf("Expected nil, got %#v", err)
	}

	if err := violationsOf(nil); err != nil {
		t.Errorf("Expected nil, got %#v", err)
	}
}

func Test_MessageType_String(t *testing.T) {
	tests := map[MessageType]string{
		MessageSearchRequest:  "M-SEARCH",
		MessageSearchResponse: "search response",
		MessageAlive:          "ssdp:alive",
		MessageByeBye:         "ssdp:byebye",
		MessageUpdate:         "ssdp:update",
		0:                     "unknown",
	}

	for messageType, expected := range tests {
		if got := messageType.String(); got != expected {
			t.Errorf("Expected %q, got %q", expected, got)
		}
	}
}

func FuzzDecode(f *testing.F) {
	seeds := []string{
		udaSearch,
		udaUnicastSearch,
		uda20Search,
		udaSearchResponse,
		udaAlive,
		udaByeBye,
		udaUpdate,
		foldedSearch,
	}

	for _, seed := range seeds {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		message, err := Decode(data)
		if err != nil {
			return
		}

		first, err := message.MarshalBinary()
		if err != nil {
			return
		}

		again, err := Decode(first)
		if err != nil {
			t.Fatalf("Failed to decode what the codec wrote: %v\n%q", err, first)
		}

		second, err := again.MarshalBinary()
		if err != nil {
			t.Fatalf("Failed to encode what the codec read back: %v", err)
		}

		if !bytes.Equal(first, second) {
			t.Fatalf("Expected a stable encoding, got %q and %q", first, second)
		}
	})
}

func BenchmarkDecode(b *testing.B) {
	data := []byte(udaSearchResponse)

	b.ReportAllocs()

	for b.Loop() {
		if _, err := Decode(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalSearch(b *testing.B) {
	search := SearchRequest{
		Host:    MulticastAddress,
		Target:  newRootDeviceTarget(),
		MaxWait: maxWaitFloor,
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := search.MarshalBinary(); err != nil {
			b.Fatal(err)
		}
	}
}
