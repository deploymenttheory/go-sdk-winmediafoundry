// Example 10_swdl_get: scrapes Microsoft's Windows 11 software-download pages
// with softwaredownload.Get and prints the product editions they advertise —
// each with the product-edition id used to resolve a download link. No session
// is established and nothing is downloaded.
//
// Run:
//
//	go run ./examples/10_swdl_get
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload"
)

func main() {
	ctx := context.Background()

	client, err := softwaredownload.NewClient()
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}

	// Get performs the scrape only (both the x64 and Arm64 pages by default).
	catalog, _, err := client.Get(ctx)
	if err != nil {
		log.Fatalf("Get: %v", err)
	}

	fmt.Printf("Scraped %d product editions\n\n", len(catalog.Products))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "EDITION ID\tARCH\tNAME")
	for _, p := range catalog.Products {
		fmt.Fprintf(w, "%s\t%s\t%s\n", p.EditionID, p.Arch, p.Name)
	}
	w.Flush()
}
