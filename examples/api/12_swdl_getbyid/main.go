// Example 12_swdl_getbyid: resolves a signed ISO download link for a specific
// product-edition id with softwaredownload.GetByID. Edition ids come from Get/
// List (example 10/11); "3324" is the Windows 11 Arm64 multi-edition ISO at the
// time of writing. Pass a different id as the first argument.
//
// Run:
//
//	go run ./examples/12_swdl_getbyid [editionID]
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload"
	sdapi "github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/api/softwaredownload"
)

func main() {
	editionID := "3324" // Windows 11 (multi-edition ISO for Arm64)
	if len(os.Args) > 1 {
		editionID = os.Args[1]
	}

	ctx := context.Background()
	client, err := softwaredownload.NewClient()
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}

	// Resolve only (no download). WithLanguage selects the SKU; the default is
	// "English (United States)".
	link, _, err := client.GetByID(ctx, editionID,
		sdapi.WithLanguage("English (United States)"))
	if err != nil {
		log.Fatalf("GetByID: %v", err)
	}

	fmt.Printf("Edition:  %s\n", link.Product.EditionID)
	fmt.Printf("File:     %s\n", link.FileName)
	fmt.Printf("Arch:     %s\n", link.Arch)
	fmt.Printf("Language: %s (SKU %s)\n", link.Language.LocalizedName, link.Language.SKUID)
	fmt.Printf("Expires:  %s\n", link.ExpiresAt.Format("2006-01-02 15:04 MST"))
	fmt.Printf("URL:      %s\n", link.URL)
}
