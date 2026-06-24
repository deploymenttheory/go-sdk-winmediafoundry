// Example 11_swdl_list: lists the available Windows 11 ARM64 product editions
// with softwaredownload.List (the flat-slice convenience over Get), narrowing
// the scrape to a single architecture via WithArch.
//
// Run:
//
//	go run ./examples/11_swdl_list
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload"
	sdapi "github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/api/softwaredownload"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/constants"
)

func main() {
	ctx := context.Background()

	client, err := softwaredownload.NewClient()
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}

	products, _, err := client.List(ctx, sdapi.WithArch(constants.ArchARM64))
	if err != nil {
		log.Fatalf("List: %v", err)
	}

	fmt.Printf("%d ARM64 product edition(s):\n", len(products))
	for _, p := range products {
		fmt.Printf("  [%s] %s\n", p.EditionID, p.Name)
	}
}
