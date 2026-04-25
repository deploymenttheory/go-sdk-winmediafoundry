// 03_get_build demonstrates GET /v1/builds/:uuid.
//
// It retrieves a single build record by UUID and prints the complete catalog
// entry as formatted JSON. The UUID is the Windows Update identity key returned
// by the SyncUpdates SOAP call (a standard UUID string, e.g.
// "038c7416-2aa2-4174-85a2-158aa9b11289").
//
// Usage (plain HTTP):
//
//	go run ./examples/03_get_build \
//	  --server http://localhost:8080 \
//	  --uuid 038c7416-2aa2-4174-85a2-158aa9b11289
//
// Usage (mTLS):
//
//	go run ./examples/03_get_build \
//	  --server https://localhost:8443 \
//	  --cert certs/client.crt --key certs/client.key --ca certs/ca.crt \
//	  --uuid 038c7416-2aa2-4174-85a2-158aa9b11289
package main

import (
	"context"
	"encoding/json"
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

	build, err := client.Builds.Get(ctx, *uuid)
	if err != nil {
		log.Fatalf("get build: %v", err)
	}

	fmt.Printf("Build: %s\n", build.Title)
	fmt.Printf("  uuid:         %s\n", build.UUID)
	fmt.Printf("  revision:     %d\n", build.Revision)
	fmt.Printf("  build:        %s\n", build.Build)
	fmt.Printf("  arch:         %s\n", build.Arch)
	fmt.Printf("  ring:         %s\n", build.Ring)
	fmt.Printf("  branch:       %s\n", build.Branch)
	fmt.Printf("  sku:          %d\n", build.SKU)
	fmt.Printf("  is_stable:    %v\n", build.IsStable)
	fmt.Printf("  is_insider:   %v\n", build.IsInsider)
	fmt.Printf("  discovered:   %s\n", build.DiscoveredAt.Format(time.RFC3339))
	fmt.Printf("  updated:      %s\n", build.UpdatedAt.Format(time.RFC3339))
	fmt.Println()
	fmt.Println("--- Full JSON ---")

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(build); err != nil {
		log.Fatal(err)
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
