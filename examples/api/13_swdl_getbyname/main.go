// Example 13_swdl_getbyname: resolves a signed ISO download link by edition name
// with softwaredownload.GetByName. The name is matched case-insensitively
// against the scraped edition labels, so "Arm64" selects
// "Windows 11 (multi-edition ISO for Arm64)".
//
// Run:
//
//	go run ./examples/13_swdl_getbyname [nameSubstring]
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload"
)

func main() {
	name := "Arm64"
	if len(os.Args) > 1 {
		name = os.Args[1]
	}

	ctx := context.Background()
	client, err := softwaredownload.NewClient()
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}

	link, _, err := client.GetByName(ctx, name)
	if err != nil {
		log.Fatalf("GetByName: %v", err)
	}

	fmt.Printf("Matched edition %q [%s]\n", link.Product.Name, link.Product.EditionID)
	fmt.Printf("  file:    %s (%s)\n", link.FileName, link.Arch)
	fmt.Printf("  expires: %s\n", link.ExpiresAt.Format("2006-01-02 15:04 MST"))
	fmt.Printf("  url:     %s\n", link.URL)
}
