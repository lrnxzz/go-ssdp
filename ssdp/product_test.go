package ssdp

import (
	"errors"
	"fmt"
	"testing"
)

func Test_ParseProductTokens(t *testing.T) {
	tests := map[string]string{
		exampleServer: "unix/5.1 | 1.1 | MyProduct/1.0",

		"Linux/2.6, UPnP/1.0, Product/1.2": "Linux/2.6 | 1.0 | Product/1.2",

		"Linux/3.14 UPnP/1.0 Portable SDK for UPnP devices/1.6.19": "Linux/3.14 | 1.0 | Portable SDK for UPnP devices/1.6.19",

		"Microsoft-Windows/10.0 UPnP/1.0 UPnP-Device-Host/1.0": "Microsoft-Windows/10.0 | 1.0 | UPnP-Device-Host/1.0",

		"UPnP/2.0":         " | 2.0 | ",
		"UPnP/1.1 Only/1":  " | 1.1 | Only/1",
		"Only/1 UPnP/1.1":  "Only/1 | 1.1 | ",
		"Linux UPnP/1.0 X": "Linux | 1.0 | X",
	}

	for value, expected := range tests {
		tokens, err := ParseProductTokens(value)
		if err != nil {
			t.Errorf("Failed to parse %q: %v", value, err)

			continue
		}

		got := fmt.Sprintf("%s | %s | %s", tokens.System(), tokens.Upnp(), tokens.Product())
		if got != expected {
			t.Errorf("Expected ParseProductTokens(%q) = %q, got %q", value, expected, got)
		}
	}
}

func Test_ParseProductTokens_RejectsAFieldWithoutUpnpVersion(t *testing.T) {
	values := []string{
		"",
		"Linux/3.14",
		"UPnP",
		"Portable SDK for UPnP devices/1.6",
		"UPnP/x",
		"UPnP/1",
	}

	for _, value := range values {
		if _, err := ParseProductTokens(value); !errors.Is(err, ErrMalformed) {
			t.Errorf("Expected ParseProductTokens(%q) to be malformed, got %v", value, err)
		}
	}
}

func Test_ProductTokens_KeepTheTextTheyWereReadFrom(t *testing.T) {
	values := []string{
		exampleServer,
		"Linux/2.6, UPnP/1.0, Product/1.2",
	}

	for _, value := range values {
		if got := mustProductTokens(t, value).String(); got != value {
			t.Errorf("Expected %q to go back out unchanged, got %q", value, got)
		}
	}
}

func Test_NewProductTokens_WritesTheUdaLayout(t *testing.T) {
	tests := map[string]ProductTokens{
		exampleServer:                NewProductTokens(NewProduct("unix", "5.1"), Version11, NewProduct("MyProduct", "1.0")),
		"UPnP/2.0":                   NewProductTokens(Product{}, Version20, Product{}),
		"Linux UPnP/1.0 goku-tv/1.0": NewProductTokens(NewProduct("Linux", ""), Version10, NewProduct("goku-tv", "1.0")),
	}

	for expected, tokens := range tests {
		if got := tokens.String(); got != expected {
			t.Errorf("Expected %q, got %q", expected, got)
		}
	}
}

func Test_NewProductTokens_ParseBackToThemselves(t *testing.T) {
	tokens := NewProductTokens(NewProduct("unix", "5.1"), Version11, NewProduct("MyProduct", "1.0"))

	if parsed := mustProductTokens(t, tokens.String()); parsed != tokens {
		t.Errorf("Expected %+v, got %+v", tokens, parsed)
	}
}

func Test_ReadProductTokens_KeepsWhatItCannotParse(t *testing.T) {
	tokens := readProductTokens("Linux/3.14")

	if tokens.String() != "Linux/3.14" || tokens.IsZero() {
		t.Errorf("Expected the raw text to be kept, got %q", tokens)
	}

	if !tokens.Upnp().IsZero() {
		t.Errorf("Expected no UPnP version, got %s", tokens.Upnp())
	}

	if !readProductTokens("").IsZero() {
		t.Error("Expected an empty field to give zero product tokens")
	}
}

func Test_Product(t *testing.T) {
	tests := map[Product]string{
		NewProduct("Linux", "3.14"): "Linux/3.14",
		NewProduct("Linux", ""):     "Linux",
		NewProduct("", ""):          "",
	}

	for product, expected := range tests {
		if got := product.String(); got != expected {
			t.Errorf("Expected %q, got %q", expected, got)
		}
	}

	product := NewProduct("MyProduct", "1.0")

	if product.Name() != "MyProduct" || product.Version() != "1.0" || product.IsZero() {
		t.Errorf("Expected MyProduct 1.0, got %s %s", product.Name(), product.Version())
	}
}
