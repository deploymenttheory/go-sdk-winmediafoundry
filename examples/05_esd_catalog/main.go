// Example 05_esd_catalog: fetches Microsoft's Windows installation ESD catalog
// (Media Creation Tool products.cab), decompresses it in pure Go (LZX), and
// lists the en-us x64 editions with their direct, non-expiring download URLs.
//
// Run:
//
//	go run ./examples/05_esd_catalog
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"github.com/deploymenttheory/winmediafoundry/esd"
	esdapi "github.com/deploymenttheory/winmediafoundry/esd/api/esd"
)

func main() {
	ctx := context.Background()

	client, err := esd.NewClient()
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}

	catalog, _, err := client.Catalog(ctx, esdapi.WithProduct(esdapi.Windows11))
	if err != nil {
		log.Fatalf("Catalog: %v", err)
	}

	fmt.Printf("Parsed %d ESD images (%d editions, %d languages, %d architectures)\n\n",
		len(catalog.Images), len(catalog.Editions()), len(catalog.Languages()),
		len(catalog.Architectures()))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "EDITION\tARCH\tSIZE (GB)\tSHA1")
	for _, img := range catalog.Filter("", "x64", "en-us") {
		fmt.Fprintf(w, "%s\t%s\t%.2f\t%s\n",
			img.Edition, img.Architecture, float64(img.SizeBytes)/1e9, img.SHA1)
	}
	w.Flush()

	if pro := catalog.Filter("Professional", "x64", "en-us"); len(pro) > 0 {
		fmt.Printf("\nProfessional x64 en-us download URL:\n%s\n", pro[0].URL)
	}
}
