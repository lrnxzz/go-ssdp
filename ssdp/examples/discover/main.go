// Discover finds the UPnP media servers on the network, fetches every description at once and prints
// what each one says about its device.
package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"maps"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"sync"
	"time"

	"github.com/lrnxzz/go-ssdp/ssdp"
)

type description struct {
	Device device `xml:"device"`
}

type device struct {
	Udn          string `xml:"UDN"`
	FriendlyName string `xml:"friendlyName"`
	Manufacturer string `xml:"manufacturer"`
	ModelName    string `xml:"modelName"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	locations := make(map[string]string)

	for answer, err := range ssdp.Search(ctx, "urn:schemas-upnp-org:device:MediaServer:1", 3*time.Second) {
		if err != nil {
			log.Fatal(err)
		}

		locations[answer.ServiceName.Uuid()] = answer.Location
	}

	uuids := slices.Sorted(maps.Keys(locations))
	devices := make([]device, len(uuids))
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	var fetches sync.WaitGroup

	for index, uuid := range uuids {
		fetches.Go(func() {
			described, err := describe(ctx, client, locations[uuid])
			if err != nil {
				log.Printf("uuid:%s: %v", uuid, err)

				return
			}

			if described.Udn != "uuid:"+uuid {
				log.Printf("uuid:%s: description at %s names %s", uuid, locations[uuid], described.Udn)
			}

			devices[index] = described
		})
	}

	fetches.Wait()

	for index, uuid := range uuids {
		fmt.Printf("%-32s %-20s %-20s %s\n", devices[index].FriendlyName, devices[index].Manufacturer, devices[index].ModelName, locations[uuid])
	}
}

func describe(ctx context.Context, client *http.Client, location string) (device, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return device{}, err
	}

	resp, err := client.Do(request)
	if err != nil {
		return device{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return device{}, fmt.Errorf("description at %s answered %s", location, resp.Status)
	}

	var described description
	if err := xml.NewDecoder(resp.Body).Decode(&described); err != nil {
		return device{}, fmt.Errorf("description at %s: %w", location, err)
	}

	return described.Device, nil
}
