// Monitor keeps an inventory of the UPnP devices on the network and prints every change to it until
// interrupted.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/lrnxzz/go-ssdp/ssdp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	controlPoint, err := ssdp.NewControlPoint(ssdp.ControlPointConfig{})
	if err != nil {
		log.Fatal(err)
	}

	online := make(map[string]ssdp.Presence)

	for event, err := range controlPoint.Watch(ctx, ssdp.NewAllTarget()) {
		if err != nil {
			log.Fatal(err)
		}

		presence := event.Presence

		switch event.Type {
		case ssdp.EventAppeared, ssdp.EventChanged, ssdp.EventRebooted:
			online[presence.Uuid] = presence
		case ssdp.EventLeft, ssdp.EventExpired:
			delete(online, presence.Uuid)
		}

		fmt.Printf("%s  %-8s  %-40s  %-18s  UDA %-4s  %d online\n",
			time.Now().Format(time.TimeOnly),
			event.Type,
			presence.Uuid,
			presence.From.Addr(),
			presence.Server.Upnp(),
			len(online),
		)
	}
}
