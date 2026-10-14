// Conformance searches the network and reports, for every device that answers, the UPnP Device
// Architecture version it declares and each MUST of that version its answers break.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"os"
	"os/signal"
	"slices"
	"time"

	"github.com/lrnxzz/go-ssdp/ssdp"
)

type report struct {
	from       string
	server     ssdp.ProductTokens
	answers    int
	violations []string
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	controlPoint, err := ssdp.NewControlPoint(ssdp.ControlPointConfig{})
	if err != nil {
		log.Fatal(err)
	}

	reports := make(map[string]report)

	for answer, err := range controlPoint.Search(ctx, ssdp.NewAllTarget(), 3*time.Second) {
		if err != nil {
			log.Fatal(err)
		}

		uuid := answer.Message.ServiceName.Uuid()

		current := reports[uuid]
		current.from = answer.From.Addr().String()
		current.server = answer.Message.Server
		current.answers++

		if violations, ok := errors.AsType[ssdp.Violations](answer.Message.Check()); ok {
			for _, violation := range violations {
				if !slices.Contains(current.violations, violation.Error()) {
					current.violations = append(current.violations, violation.Error())
				}
			}
		}

		reports[uuid] = current
	}

	for _, uuid := range slices.Sorted(maps.Keys(reports)) {
		current := reports[uuid]

		declared := "1.0 (undeclared)"
		if !current.server.Upnp().IsZero() {
			declared = current.server.Upnp().String()
		}

		fmt.Printf("uuid:%s\n  from     %s\n  server   %s\n  UDA      %s\n  answers  %d\n", uuid, current.from, current.server, declared, current.answers)

		if len(current.violations) == 0 {
			fmt.Println("  conforms")
		}

		for _, violation := range current.violations {
			fmt.Printf("  breaks   %s\n", violation)
		}
	}
}
