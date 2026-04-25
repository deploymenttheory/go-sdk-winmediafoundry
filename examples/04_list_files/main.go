// 04_list_files demonstrates GET /v1/builds/:uuid/files.
//
// It retrieves file metadata for a build from the catalog — name, size,
// SHA1 hash, and file type — without resolving live CDN URLs. Use this to
// inspect what files are available before committing to URL resolution
// (which has a ~12 minute expiry window).
//
// Filter by extension with --ext to narrow the list (e.g. --ext .esd shows
// only Windows installation images).
//
// Usage (plain HTTP):
//
//	go run ./examples/04_list_files \
//	  --server http://localhost:8080 \
//	  --uuid 038c7416-2aa2-4174-85a2-158aa9b11289
//
//	go run ./examples/04_list_files \
//	  --server http://localhost:8080 \
//	  --uuid 038c7416-2aa2-4174-85a2-158aa9b11289 \
//	  --ext .esd
//
// Usage (mTLS):
//
//	go run ./examples/04_list_files \
//	  --server https://localhost:8443 \
//	  --cert certs/client.crt --key certs/client.key --ca certs/ca.crt \
//	  --uuid 038c7416-2aa2-4174-85a2-158aa9b11289
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/deploymenttheory/go-sdk-uupdump/sdk"
)

func main() {
	server := flag.String("server", "http://localhost:8080", "winupdate server base URL")
	cert := flag.String("cert", "", "client certificate file (omit for plain HTTP)")
	key := flag.String("key", "", "client private key file")
	ca := flag.String("ca", "", "CA certificate file")
	uuid := flag.String("uuid", "", "build UUID (required)")
	ext := flag.String("ext", "", "filter by file extension, e.g. .esd or .cab")
	flag.Parse()

	if *uuid == "" {
		fmt.Fprintln(os.Stderr, "error: --uuid is required")
		flag.Usage()
		os.Exit(1)
	}

	client, err := newClient(*server, *cert, *key, *ca)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	q := client.Files.QueryFiles(*uuid)
	if *ext != "" {
		q = q.ByExtension(*ext)
	}
	files, err := q.Execute(ctx)
	if err != nil {
		log.Fatalf("list files: %v", err)
	}

	if len(files) == 0 {
		fmt.Printf("No files found for build %s", *uuid)
		if *ext != "" {
			fmt.Printf(" with extension %s", *ext)
		}
		fmt.Println()
		fmt.Println("Tip: files are stored at ingest time — run 01_fetch_updates first.")
		return
	}

	var totalBytes int64
	fmt.Printf("%-60s  %12s  %-10s  %s\n", "Name", "Size", "Type", "SHA1")
	fmt.Printf("%-60s  %12s  %-10s  %s\n",
		"------------------------------------------------------------",
		"------------", "----------", "----------------------------------------")

	for _, f := range files {
		fmt.Printf("%-60s  %12s  %-10s  %s\n",
			f.Name, formatBytes(f.SizeBytes), string(f.FileType), f.SHA1)
		totalBytes += f.SizeBytes
	}

	fmt.Printf("\n%d file(s)  —  total size: %s\n", len(files), formatBytes(totalBytes))
	fmt.Println("\nTip: run 05_resolve_cdn_urls to get live Microsoft CDN download URLs.")
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

func newClient(server, cert, key, ca string) (*sdk.Client, error) {
	opts := []sdk.ClientOption{
		sdk.WithBaseURL(server),
		sdk.WithTimeout(30 * time.Second),
		sdk.WithRetryCount(2),
	}
	if cert != "" {
		opts = append(opts, sdk.WithMTLS(cert, key, ca))
	} else {
		opts = append(opts, sdk.WithInsecureSkipVerify())
	}
	return sdk.NewClient(opts...)
}
