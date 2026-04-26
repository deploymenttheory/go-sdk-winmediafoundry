// 01_fetch_updates demonstrates POST /v1/updates/fetch.
//
// It triggers a live SyncUpdates SOAP call through the winupdate server,
// which queries Microsoft's Windows Update endpoints and stores any newly
// discovered or updated builds in the catalog.
//
// The --check-build flag controls what OS version the WU client claims to be
// running. Setting it to an old Windows 10 Insider build (10.0.16251.0) causes
// Windows Update to offer the current stable Windows 11 release as an upgrade,
// returning a leaf update with a proper UpdateID that can be resolved to CDN
// file URLs via GetExtendedUpdateInfo2.
//
// Usage (plain HTTP):
//
//	go run ./examples/01_fetch_updates \
//	  --server http://localhost:8080 \
//	  --arch amd64 --ring Retail --check-build 10.0.16251.0
//
// Usage (mTLS):
//
//	go run ./examples/01_fetch_updates \
//	  --server https://localhost:8443 \
//	  --cert certs/client.crt --key certs/client.key --ca certs/ca.crt \
//	  --arch amd64 --ring Retail --check-build 10.0.16251.0
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/deploymenttheory/go-sdk-windowsuup/sdk"
)

func main() {
	server := flag.String("server", "http://localhost:8080", "winupdate server base URL")
	cert := flag.String("cert", "", "client certificate file (omit for plain HTTP)")
	key := flag.String("key", "", "client private key file")
	ca := flag.String("ca", "", "CA certificate file")
	arch := flag.String("arch", "amd64", "architecture: amd64, arm64, x86")
	ring := flag.String("ring", "Retail", "ring: Retail, ReleasePreview, Beta, Dev, Canary")
	checkBuild := flag.String("check-build", "10.0.16251.0",
		"OS version the WU client claims to be on; an old build causes WU to offer the current stable release as an upgrade")
	flag.Parse()

	client, err := newClient(*server, *cert, *key, *ca)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	fmt.Printf("Querying Windows Update: arch=%s ring=%s check-build=%s\n", *arch, *ring, *checkBuild)

	result, err := client.Updates.Fetch(ctx, sdk.FetchRequest{
		Arch:       *arch,
		Ring:       *ring,
		Flight:     "Active",
		CheckBuild: *checkBuild,
	})
	if err != nil {
		log.Fatalf("fetch updates: %v", err)
	}

	fmt.Printf("\ntotal=%d  new=%d  updated=%d\n\n", result.Total, len(result.NewBuilds), len(result.Updated))

	if len(result.NewBuilds) > 0 {
		fmt.Println("=== New builds ===")
		for _, b := range result.NewBuilds {
			fmt.Printf("  uuid=%-38s  build=%-14s  arch=%-6s  ring=%s\n",
				b.UUID, b.Build, b.Arch, b.Ring)
			if b.Title != "" {
				fmt.Printf("  title: %s\n", b.Title)
			}
		}
	}

	if len(result.Updated) > 0 {
		fmt.Println("\n=== Updated builds ===")
		for _, b := range result.Updated {
			fmt.Printf("  uuid=%-38s  build=%-14s  arch=%-6s  ring=%s\n",
				b.UUID, b.Build, b.Arch, b.Ring)
		}
	}

	if result.Total == 0 {
		fmt.Println("No builds returned — the catalog is already up to date for this arch/ring.")
		fmt.Println("Tip: try a different ring (Dev, Beta) or verify --check-build is set to an old build.")
		return
	}

	fmt.Println("\n--- Raw result ---")
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		log.Fatal(err)
	}
}

func newClient(server, cert, key, ca string) (*sdk.Client, error) {
	opts := []sdk.ClientOption{
		sdk.WithBaseURL(server),
		sdk.WithTimeout(3 * time.Minute),
		sdk.WithRetryCount(2),
	}
	if cert != "" {
		opts = append(opts, sdk.WithMTLS(cert, key, ca))
	} else {
		opts = append(opts, sdk.WithInsecureSkipVerify())
	}
	return sdk.NewClient(opts...)
}
