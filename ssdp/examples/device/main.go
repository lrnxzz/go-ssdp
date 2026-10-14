// Device runs the discovery side of a UPnP root device: it serves its description over HTTP, announces
// itself and answers searches until interrupted, then cancels its announcements.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"text/template"

	"github.com/lrnxzz/go-ssdp/ssdp"
)

const uuid = "2fac1234-31f8-11b4-a222-08002b34c003"

const configId = 1

var description = template.Must(template.New("description").Parse(`<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0" configId="{{.ConfigId}}">
  <specVersion>
    <major>1</major>
    <minor>1</minor>
  </specVersion>
  <device>
    <deviceType>{{.DeviceType}}</deviceType>
    <friendlyName>go-ssdp example</friendlyName>
    <manufacturer>go-ssdp</manufacturer>
    <modelName>device</modelName>
    <UDN>uuid:{{.Uuid}}</UDN>
  </device>
</root>
`))

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	deviceType, err := ssdp.NewDeviceTypeTarget(ssdp.StandardDomain, "Basic", 1)
	if err != nil {
		log.Fatal(err)
	}

	route, err := net.Dial("udp4", ssdp.MulticastAddress)
	if err != nil {
		log.Fatal(err)
	}

	host := route.LocalAddr().(*net.UDPAddr).IP
	_ = route.Close()

	listener, err := net.Listen("tcp4", net.JoinHostPort(host.String(), "0"))
	if err != nil {
		log.Fatal(err)
	}

	location := fmt.Sprintf("http://%s/description.xml", listener.Addr())

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)

			err := description.Execute(w, map[string]any{
				"ConfigId":   configId,
				"DeviceType": deviceType,
				"Uuid":       uuid,
			})
			if err != nil {
				log.Print(err)
			}
		}),
	}

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	device, err := ssdp.NewDevice(ssdp.DeviceConfig{
		Advertisements: ssdp.NewRootDeviceAdvertisements(uuid, deviceType),
		Location:       location,
		Server:         ssdp.NewProductTokens(ssdp.NewProduct("Linux", ""), ssdp.Version11, ssdp.NewProduct("go-ssdp-example", "1.0")),
		MaxAge:         ssdp.MaxAgeFloor,
		BootId:         ssdp.NewOptional(1),
		ConfigId:       ssdp.NewOptional(configId),
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("announcing uuid:%s at %s\n", uuid, location)

	<-ctx.Done()

	if err := device.Close(); err != nil {
		log.Print(err)
	}

	if err := server.Shutdown(context.Background()); err != nil {
		log.Print(err)
	}
}
