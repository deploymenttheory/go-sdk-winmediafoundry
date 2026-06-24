// Example 14_swdl_download: resolves and downloads an official Windows 11 ARM64
// ISO. It shows both downloading styles softwaredownload offers:
//
//   - inline: GetByName with WithDownloadDir streams the ISO as part of resolution.
//   - explicit: Download takes an already-resolved DownloadLink.
//
// The ISO is several gigabytes, so the download only runs when -out is given:
//
//	go run ./examples/14_swdl_download -out /tmp/win
//
// Without -out it resolves and prints the link only.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload"
	sdapi "github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/api/softwaredownload"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/constants"
)

func main() {
	out := flag.String("out", "", "directory to download the ISO into (multi-GB); resolve-only when empty")
	flag.Parse()

	ctx := context.Background()
	client, err := softwaredownload.NewClient()
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}

	if *out == "" {
		// Resolve-only: print the signed link.
		link, _, err := client.GetByName(ctx, "Arm64", sdapi.WithArch(constants.ArchARM64))
		if err != nil {
			log.Fatalf("GetByName: %v", err)
		}
		fmt.Printf("Resolved %s (%s)\n  %s\n", link.FileName, link.Arch, link.URL)
		fmt.Println("\nPass -out <dir> to download it.")
		return
	}

	// Inline download: WithDownloadDir streams the ISO during resolution and a
	// terminal progress bar is rendered via WithProgress(nil) (writes to stderr).
	link, _, err := client.GetByName(ctx, "Arm64",
		sdapi.WithArch(constants.ArchARM64),
		sdapi.WithDownloadDir(*out),
		sdapi.WithProgress(nil),
	)
	if err != nil {
		log.Fatalf("GetByName(download): %v", err)
	}
	fmt.Printf("\nDownloaded to %s\n", link.LocalPath)

	// Explicit download form: resolve first, then call Download with the link.
	// (Skipped here since the file is already present; shown for reference.)
	if false {
		l2, _, err := client.GetByName(ctx, "Arm64")
		if err != nil {
			log.Fatal(err)
		}
		if _, err := client.Download(ctx, *l2, *out, sdapi.WithProgress(nil)); err != nil {
			log.Fatal(err)
		}
	}
}
