// Example 02_get_files: fetches the file list for the latest amd64 Retail build,
// resolves live CDN download URLs, and filters to en-us Professional ESD files.
//
// Run:
//
//	go run ./examples/02_get_files
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"text/tabwriter"
	"time"

	buildsapi "github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/api/builds"
	filesapi "github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/api/files"
	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/constants"

	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup"
)

func main() {
	ctx := context.Background()

	client, err := windowsuup.NewClient()
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}

	builds, _, err := client.Builds.FetchBuilds(ctx,
		buildsapi.WithArch(constants.ArchAMD64),
		buildsapi.WithRing(constants.RingRetail),
		buildsapi.WithSKU(constants.SKUPro),
	)
	if err != nil {
		log.Fatalf("FetchBuilds: %v", err)
	}
	if len(builds) == 0 {
		log.Fatal("no builds found")
	}

	build := builds[0]
	fmt.Printf("Build: %s  %s\n\n", build.Build, build.Title)

	files, _, err := client.Files.GetFiles(ctx, build,
		filesapi.WithCDNURLs(),
		filesapi.WithLanguage("en-us"),
		filesapi.WithEdition(constants.EditionProfessional),
	)
	if err != nil {
		log.Fatalf("GetFiles: %v", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FILE\tSIZE (MB)\tEXPIRES")
	for _, f := range files {
		sizeMB := float64(f.SizeBytes) / 1024 / 1024
		expires := ""
		if !f.ExpiresAt.IsZero() {
			expires = f.ExpiresAt.Format(time.RFC3339)
		}
		fmt.Fprintf(w, "%s\t%.1f\t%s\n", f.Name, sizeMB, expires)
	}
	w.Flush()

	fmt.Println()
	for _, f := range files {
		fmt.Printf("URL: %s\n", f.URL)
	}
}
