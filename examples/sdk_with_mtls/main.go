// sdk_with_mtls demonstrates connecting to the winupdate server with mTLS.
//
// It performs a live Windows Update query via the server and prints the
// new/updated build summary.
//
// Usage:
//
//	go run ./examples/sdk_with_mtls \
//	  --server  https://winupdate.internal:8443 \
//	  --cert    client.crt \
//	  --key     client.key \
//	  --ca      ca.crt
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
	server := flag.String("server", "https://localhost:8443", "winupdate server base URL")
	cert := flag.String("cert", "client.crt", "client certificate file")
	key := flag.String("key", "client.key", "client private key file")
	ca := flag.String("ca", "ca.crt", "CA certificate file")
	arch := flag.String("arch", "amd64", "architecture to query")
	ring := flag.String("ring", "Retail", "ring to query")
	flag.Parse()

	client, err := sdk.NewClient(
		sdk.WithBaseURL(*server),
		sdk.WithMTLS(*cert, *key, *ca),
		sdk.WithTimeout(60*time.Second),
		sdk.WithRetryCount(3),
		sdk.WithRetryWaitTime(2*time.Second),
	)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	fmt.Printf("Fetching Windows Update results: arch=%s ring=%s\n", *arch, *ring)
	result, err := client.Updates.Fetch(ctx, sdk.FetchRequest{
		Arch:   *arch,
		Ring:   *ring,
		Flight: "Active",
	})
	if err != nil {
		log.Fatalf("fetch: %v", err)
	}

	fmt.Printf("total=%d new=%d updated=%d\n",
		result.Total, len(result.NewBuilds), len(result.Updated))

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		log.Fatal(err)
	}
}
