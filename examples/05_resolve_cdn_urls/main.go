// 05_resolve_cdn_urls demonstrates GET /v1/builds/:uuid/files?with_urls=true.
//
// It calls GetExtendedUpdateInfo2 (EUI2) through the server to resolve live
// pre-signed Microsoft CDN download URLs for every file in the build. The URLs
// point to tlu.dl.delivery.mp.microsoft.com/filestreamingservice/files/ and
// expire in approximately 12 minutes — download promptly or re-resolve.
//
// The --revision flag must match the build's revision number (from 03_get_build
// or 02_list_builds). Without the correct revision EUI2 will return no URLs.
//
// Usage:
//
//	go run ./examples/05_resolve_cdn_urls \
//	  --uuid 038c7416-2aa2-4174-85a2-158aa9b11289 \
//	  --revision 1
//
//	# ESD files only:
//	go run ./examples/05_resolve_cdn_urls \
//	  --uuid 038c7416-2aa2-4174-85a2-158aa9b11289 \
//	  --revision 1 --ext .esd
//
//	go run ./examples/05_resolve_cdn_urls \
//	  --server https://wuapi.example.internal:8443 \
//	  --uuid 038c7416-2aa2-4174-85a2-158aa9b11289 \
//	  --revision 1
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/deploymenttheory/go-sdk-windowsuup/sdk"
)

func main() {
	server := flag.String("server", "https://localhost:8443", "winupdate server base URL")
	uuid := flag.String("uuid", "", "build UUID (required)")
	revision := flag.Int("revision", 0, "build revision number (required; from 03_get_build output)")
	ext := flag.String("ext", "", "filter by file extension, e.g. .esd or .cab")
	flag.Parse()

	if *uuid == "" || *revision == 0 {
		fmt.Fprintln(os.Stderr, "error: --uuid and --revision are required")
		flag.Usage()
		os.Exit(1)
	}

	client, err := newClient(*server)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	// Allow extra time — EUI2 may take several seconds.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	fmt.Printf("Resolving CDN URLs for build %s (revision %d)...\n\n", *uuid, *revision)

	q := client.Files.Query(*uuid).WithURLs(*revision)
	if *ext != "" {
		q = q.ByExtension(*ext)
	}
	files, err := q.Execute(ctx)
	if err != nil {
		log.Fatalf("resolve CDN URLs: %v", err)
	}

	if len(files) == 0 {
		fmt.Println("No files returned with CDN URLs.")
		fmt.Println("This usually means:")
		fmt.Println("  • The revision number is wrong (check 03_get_build output)")
		fmt.Println("  • The update's device attributes don't match (re-run 01_fetch_updates)")
		fmt.Println("  • The Microsoft CDN tokens have expired (wait and retry)")
		return
	}

	var withURL, withoutURL int
	for _, f := range files {
		if f.URL != "" {
			withURL++
		} else {
			withoutURL++
		}
	}

	fmt.Printf("Resolved %d CDN URL(s)  (%d file(s) without URL)\n\n", withURL, withoutURL)
	fmt.Printf("%-50s  %10s  %-26s\n", "Name", "Size", "Expires")
	fmt.Printf("%-50s  %10s  %-26s\n",
		"--------------------------------------------------", "----------", "--------------------------")

	for _, f := range files {
		if f.URL == "" {
			continue
		}
		fmt.Printf("%-50s  %10s  %-26s\n",
			f.Name, formatBytes(f.SizeBytes), f.ExpiresAt)
		fmt.Printf("  URL: %s\n\n", f.URL)
	}

	fmt.Println("CDN URLs expire in ~12 minutes. To download a file:")
	fmt.Printf("  curl -o <filename> \"<url>\"\n")
	fmt.Println("Or use example 06_download_file to stream via the server.")
}

func formatBytes(b int64) string {
	const (
		GiB = 1 << 30
		MiB = 1 << 20
		KiB = 1 << 10
	)
	switch {
	case b >= GiB:
		return fmt.Sprintf("%.2f GiB", float64(b)/GiB)
	case b >= MiB:
		return fmt.Sprintf("%.1f MiB", float64(b)/MiB)
	case b >= KiB:
		return fmt.Sprintf("%.0f KiB", float64(b)/KiB)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func newClient(server string) (*sdk.Client, error) {
	return sdk.NewClient(
		sdk.WithBaseURL(server),
		sdk.WithTimeout(2*time.Minute),
		sdk.WithRetryCount(2),
	)
}
