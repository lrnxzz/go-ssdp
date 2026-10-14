package ssdp

import (
	"bufio"
	"bytes"
	"encoding"
	"fmt"
	"maps"
	"net/http"
	"net/textproto"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

type MessageType int

const (
	MessageSearchRequest MessageType = iota + 1
	MessageSearchResponse
	MessageAlive
	MessageByeBye
	MessageUpdate
)

const (
	lineEnd           = "\r\n"
	fieldSeparator    = ": "
	directiveSplitter = ","
	directiveAssign   = "="
	startLineSpace    = " "

	violationSeparator = "; "
)

var blankLine = []byte(lineEnd + lineEnd)

var standardHeaders = canonicalNames(
	headerHost, headerMan, headerMaxWait, headerSearchTarget, headerNotificationType,
	headerNotificationSubtype, headerServiceName, headerLocation, headerServer, headerUserAgent,
	headerCacheControl, headerDate, headerExt, headerBootId, headerConfigId, headerNextBootId,
	headerSearchPort, headerSecureLocation, headerControlPointName, headerControlPointUuid, headerTcpPort,
)

func (t MessageType) String() string {
	switch t {
	case MessageSearchRequest:
		return methodSearch
	case MessageSearchResponse:
		return "search response"
	case MessageAlive:
		return subtypeAlive
	case MessageByeBye:
		return subtypeByeBye
	case MessageUpdate:
		return subtypeUpdate
	default:
		return "unknown"
	}
}

// Message is one of the five SSDP datagrams.
type Message interface {
	encoding.BinaryMarshaler

	Type() MessageType

	// Check reports every MUST the message breaks under the version it declares.
	Check() error
}

// Decode reads any SSDP datagram into the message it holds.
func Decode(data []byte) (Message, error) {
	f, err := readFrame(data)
	if err != nil {
		return nil, err
	}

	switch {
	case f.isResponse():
		return decode[SearchResponse](f)
	case f.isRequest(methodSearch):
		return decode[SearchRequest](f)
	case f.isRequest(methodNotify):
		switch subtype := f.header.Get(headerNotificationSubtype); subtype {
		case subtypeAlive:
			return decode[Alive](f)
		case subtypeByeBye:
			return decode[ByeBye](f)
		case subtypeUpdate:
			return decode[Update](f)
		default:
			return nil, fmt.Errorf("%w: NTS %q is not a notification subtype", ErrMalformed, subtype)
		}
	default:
		return nil, fmt.Errorf("%w: %q is none of the three SSDP start-lines", ErrUnexpected, strings.Join(f.startLine, startLineSpace))
	}
}

type messagePointer[T any] interface {
	*T
	read(f frame) error
}

func decode[T Message, P messagePointer[T]](f frame) (Message, error) {
	var message T
	if err := P(&message).read(f); err != nil {
		return nil, err
	}

	return message, nil
}

func unmarshal[T Message](data []byte, target *T) error {
	message, err := Decode(data)
	if err != nil {
		return err
	}

	typed, ok := message.(T)
	if !ok {
		return fmt.Errorf("%w: got %s, want %s", ErrUnexpected, message.Type(), (*target).Type())
	}

	*target = typed

	return nil
}

// Violations lists every MUST a message breaks; each one wraps ErrNonConformant.
type Violations []error

func (v Violations) Error() string {
	messages := make([]string, 0, len(v))

	for _, violation := range v {
		messages = append(messages, violation.Error())
	}

	return strings.Join(messages, violationSeparator)
}

func (v Violations) Unwrap() []error {
	return v
}

func violationsOf(errs []error) error {
	violations := make(Violations, 0, len(errs))

	for _, err := range errs {
		if err != nil {
			violations = append(violations, err)
		}
	}

	if len(violations) == 0 {
		return nil
	}

	return violations
}

func violation(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrNonConformant, fmt.Sprintf(format, args...))
}

func requiredSince(rules, since Version, present bool, header string) error {
	if present || !rules.AtLeast(since) {
		return nil
	}

	return violation("UDA %s requires %s", rules, header)
}

func canonicalNames(names ...string) []string {
	canonical := make([]string, 0, len(names))

	for _, name := range names {
		canonical = append(canonical, textproto.CanonicalMIMEHeaderKey(name))
	}

	return canonical
}

type frame struct {
	startLine []string
	header    http.Header
}

func readFrame(data []byte) (frame, error) {
	if !bytes.HasSuffix(data, blankLine) {
		data = append(bytes.Clone(data), blankLine...)
	}

	reader := textproto.NewReader(bufio.NewReader(bytes.NewReader(data)))

	line, err := reader.ReadLine()
	if err != nil {
		return frame{}, fmt.Errorf("%w: %w", ErrMalformed, err)
	}

	header, err := reader.ReadMIMEHeader()
	if err != nil {
		return frame{}, fmt.Errorf("%w: %w", ErrMalformed, err)
	}

	for _, values := range header {
		for index, value := range values {
			values[index] = strings.TrimSpace(value)
		}
	}

	return frame{
		startLine: strings.Fields(line),
		header:    http.Header(header),
	}, nil
}

func (f frame) isResponse() bool {
	return len(f.startLine) >= 2 && f.startLine[0] == protocolName && f.startLine[1] == statusOk
}

func (f frame) isRequest(method string) bool {
	return len(f.startLine) == 3 && f.startLine[0] == method && f.startLine[1] == requestTarget && f.startLine[2] == protocolName
}

func (f frame) has(name string) bool {
	_, ok := f.header[textproto.CanonicalMIMEHeaderKey(name)]

	return ok
}

func (f frame) required(name string) (string, error) {
	value := f.header.Get(name)
	if value == "" {
		return "", fmt.Errorf("%w: %s is missing", ErrMalformed, name)
	}

	return value, nil
}

func (f frame) target(name string) (Target, error) {
	value, err := f.required(name)
	if err != nil {
		return Target{}, err
	}

	return ParseTarget(value)
}

func (f frame) serviceName() (ServiceName, error) {
	value, err := f.required(headerServiceName)
	if err != nil {
		return ServiceName{}, err
	}

	return ParseServiceName(value)
}

func (f frame) url(name string) (string, error) {
	value := f.header.Get(name)
	if value == "" {
		return "", nil
	}

	location, err := url.Parse(value)
	if err != nil || !location.IsAbs() || location.Host == "" {
		return "", fmt.Errorf("%w: %s %q is not an absolute URL", ErrMalformed, name, value)
	}

	return value, nil
}

func (f frame) counter(name string) (Optional[int], error) {
	return f.ranged(name, 0, counterCeiling)
}

func (f frame) port(name string) (Optional[int], error) {
	return f.ranged(name, portFloor, portCeiling)
}

func (f frame) ranged(name string, floor, ceiling int) (Optional[int], error) {
	value := f.header.Get(name)
	if value == "" {
		return Optional[int]{}, nil
	}

	number, err := strconv.Atoi(value)
	if err != nil || number < floor || number > ceiling {
		return Optional[int]{}, fmt.Errorf("%w: %s %q is outside %d-%d", ErrMalformed, name, value, floor, ceiling)
	}

	return NewOptional(number), nil
}

func (f frame) seconds(name string) (time.Duration, error) {
	value := f.header.Get(name)
	if value == "" {
		return 0, nil
	}

	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds < 0 {
		return 0, fmt.Errorf("%w: %s %q is not a number of seconds", ErrMalformed, name, value)
	}

	return time.Duration(seconds) * time.Second, nil
}

func (f frame) maxAge() (time.Duration, error) {
	cacheControl := f.header.Get(headerCacheControl)

	for directive := range strings.SplitSeq(cacheControl, directiveSplitter) {
		name, value, ok := strings.Cut(directive, directiveAssign)
		if !ok || !strings.EqualFold(strings.TrimSpace(name), maxAgeDirective) {
			continue
		}

		seconds, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || seconds < 0 {
			return 0, fmt.Errorf("%w: %s %q has no valid max-age", ErrMalformed, headerCacheControl, cacheControl)
		}

		return time.Duration(seconds) * time.Second, nil
	}

	return 0, nil
}

func (f frame) date() (time.Time, error) {
	value := f.header.Get(headerDate)
	if value == "" {
		return time.Time{}, nil
	}

	date, err := http.ParseTime(value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %s %q is not an HTTP date", ErrMalformed, headerDate, value)
	}

	return date, nil
}

func (f frame) productTokens(name string) ProductTokens {
	return readProductTokens(f.header.Get(name))
}

func (f frame) extensions() http.Header {
	var extensions http.Header

	for name, values := range f.header {
		if slices.Contains(standardHeaders, name) {
			continue
		}

		if extensions == nil {
			extensions = make(http.Header)
		}

		extensions[name] = slices.Clone(values)
	}

	return extensions
}

type writer struct {
	buffer bytes.Buffer
}

func newWriter() *writer {
	return &writer{}
}

func (w *writer) write(parts ...string) *writer {
	for _, part := range parts {
		w.buffer.WriteString(part)
	}

	return w
}

func (w *writer) requestLine(method string) *writer {
	return w.write(method, startLineSpace, requestTarget, startLineSpace, protocolName, lineEnd)
}

func (w *writer) statusLine() *writer {
	return w.write(protocolName, startLineSpace, statusOk, startLineSpace, reasonOk, lineEnd)
}

func (w *writer) field(name, value string) *writer {
	return w.write(name, fieldSeparator, value, lineEnd)
}

func (w *writer) optional(name, value string) *writer {
	if value == "" {
		return w
	}

	return w.field(name, value)
}

func (w *writer) number(name string, value Optional[int]) *writer {
	number, ok := value.Get()
	if !ok {
		return w
	}

	return w.field(name, strconv.Itoa(number))
}

func (w *writer) seconds(name string, value time.Duration) *writer {
	if value <= 0 {
		return w
	}

	return w.field(name, strconv.Itoa(int(value/time.Second)))
}

func (w *writer) maxAge(value time.Duration) *writer {
	if value <= 0 {
		return w
	}

	return w.field(headerCacheControl, maxAgeDirective+directiveAssign+strconv.Itoa(int(value/time.Second)))
}

func (w *writer) date(value time.Time) *writer {
	if value.IsZero() {
		return w
	}

	return w.field(headerDate, value.UTC().Format(http.TimeFormat))
}

func (w *writer) extensions(extensions http.Header) *writer {
	for _, name := range slices.Sorted(maps.Keys(extensions)) {
		for _, value := range extensions[name] {
			w.field(name, value)
		}
	}

	return w
}

func (w *writer) bytes() []byte {
	return w.write(lineEnd).buffer.Bytes()
}
