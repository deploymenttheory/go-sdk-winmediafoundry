// Example 01_fetch_builds: discovers available Windows builds from Microsoft's
// Windows Update service for amd64 Retail and Experimental (Insider Dev) rings.
//
// Run:
//
//	go run ./examples/01_fetch_builds
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	buildsapi "github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/api/builds"
	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/constants"

	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup"
)

func main() {
	ctx := context.Background()

	client, err := windowsuup.NewClient()
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "RING\tBUILD\tTITLE\tUUID\tSTABLE")

	for _, ring := range []constants.Ring{constants.RingRetail, constants.RingExperimental} {
		builds, _, err := client.Builds.FetchBuilds(ctx,
			buildsapi.WithArch(constants.ArchAMD64),
			buildsapi.WithRing(ring),
			buildsapi.WithSKU(constants.SKUPro),
		)
		if err != nil {
			log.Printf("FetchBuilds %s: %v", ring, err)
			continue
		}

		for _, b := range builds {
			stable := "no"
			if b.IsStable {
				stable = "yes"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				b.Ring, b.Build, b.Title, b.UUID, stable)
		}
	}

	w.Flush()
}
