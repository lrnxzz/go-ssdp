package ssdp

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

const (
	upnpProductName   = "UPnP"
	productSeparator  = "/"
	productDelimiters = " ,"
	tokenSeparator    = " "
)

// Product is one "name/version" token.
type Product struct {
	name    string
	version string
}

func newProduct(name, version string) Product {
	return Product{
		name:    name,
		version: version,
	}
}

func parseProduct(token string) Product {
	name, version, _ := strings.Cut(token, productSeparator)

	return newProduct(name, version)
}

func (p Product) Name() string {
	return p.name
}

func (p Product) Version() string {
	return p.version
}

func (p Product) IsZero() bool {
	return p.name == ""
}

func (p Product) String() string {
	if p.version == "" {
		return p.name
	}

	return p.name + productSeparator + p.version
}

// ProductTokens is a SERVER or USER-AGENT field, kept as the text it was read from.
type ProductTokens struct {
	system  Product
	upnp    Version
	product Product
	raw     string
}

func newProductTokens(system Product, upnp Version, product Product) ProductTokens {
	tokens := ProductTokens{
		system:  system,
		upnp:    upnp,
		product: product,
	}

	tokens.raw = tokens.format()

	return tokens
}

// ParseProductTokens reads the operating system, UPnP version and product of a SERVER or USER-AGENT field.
func ParseProductTokens(value string) (ProductTokens, error) {
	tokens := strings.FieldsFunc(value, isProductDelimiter)

	for index, token := range tokens {
		product := parseProduct(token)
		if product.name != upnpProductName || product.version == "" {
			continue
		}

		upnp, err := ParseVersion(product.version)
		if err != nil {
			return ProductTokens{}, err
		}

		return ProductTokens{
			system:  joinProduct(tokens[:index]),
			upnp:    upnp,
			product: joinProduct(tokens[index+1:]),
			raw:     value,
		}, nil
	}

	return ProductTokens{}, fmt.Errorf("%w: %q has no UPnP/major.minor token", ErrMalformed, value)
}

// defaultProductTokens fills SERVER and USER-AGENT when a configuration leaves them empty, in the
// "OS/version UPnP/1.1 product/version" form of §1.2.2.
func defaultProductTokens() ProductTokens {
	version := ""

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, module := range append(info.Deps, &info.Main) {
			if module.Path == "github.com/lrnxzz/go-ssdp" && module.Version != "(devel)" {
				version = strings.TrimPrefix(module.Version, "v")
			}
		}
	}

	return newProductTokens(newProduct(runtime.GOOS, systemVersion()), Version11, newProduct("go-ssdp", version))
}

func readProductTokens(value string) ProductTokens {
	tokens, err := ParseProductTokens(value)
	if err != nil {
		return ProductTokens{
			raw: value,
		}
	}

	return tokens
}

func isProductDelimiter(char rune) bool {
	return strings.ContainsRune(productDelimiters, char)
}

func joinProduct(tokens []string) Product {
	if len(tokens) == 0 {
		return Product{}
	}

	return parseProduct(strings.Join(tokens, tokenSeparator))
}

func (t ProductTokens) System() Product {
	return t.system
}

// Upnp is zero when the field declares no UPnP version.
func (t ProductTokens) Upnp() Version {
	return t.upnp
}

func (t ProductTokens) Product() Product {
	return t.product
}

func (t ProductTokens) IsZero() bool {
	return t.raw == ""
}

func (t ProductTokens) String() string {
	return t.raw
}

func (t ProductTokens) format() string {
	tokens := make([]string, 0, 3)

	if !t.system.IsZero() {
		tokens = append(tokens, t.system.String())
	}

	tokens = append(tokens, newProduct(upnpProductName, t.upnp.String()).String())

	if !t.product.IsZero() {
		tokens = append(tokens, t.product.String())
	}

	return strings.Join(tokens, tokenSeparator)
}
