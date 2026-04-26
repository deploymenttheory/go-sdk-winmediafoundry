// Example 03_download: fetches the latest amd64 Retail build, resolves CDN URLs
// for en-us Professional ESD files, and downloads them concurrently to ./downloads/.
//
// Run:
//
//	go run ./examples/03_download
package main

import (
	"context"
	"fmt"
	"log"

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
	fmt.Printf("Downloading files for: %s  %s\n\n", build.Build, build.Title)

	files, _, err := client.Files.GetFiles(ctx, build,
		filesapi.WithCDNURLs(),
		filesapi.WithLanguage("en-us"),
		filesapi.WithEdition(constants.EditionProfessional),
		filesapi.WithExtension(".esd"),
	)
	if err != nil {
		log.Fatalf("GetFiles: %v", err)
	}

	fmt.Printf("Downloading %d file(s) to ./downloads/ ...\n", len(files))

	if err := client.Download.DownloadFiles(ctx, files, "./downloads", 4); err != nil {
		log.Fatalf("DownloadFiles: %v", err)
	}

	fmt.Println("Done.")
}
