// 09_feed_stream demonstrates GET /v1/feed/stream.
//
// It connects to the Server-Sent Events (SSE) live feed and prints each
// build event as it arrives. The stream carries two event types:
//   - build_discovered — a new build UUID was seen for the first time
//   - build_updated    — an existing build's metadata was refreshed
//
// The program runs until interrupted (Ctrl-C / SIGTERM). The server sends a
// keep-alive comment every 15 seconds so the connection does not time out.
//
// Usage:
//
//	go run ./examples/09_feed_stream
//
//	go run ./examples/09_feed_stream \
//	  --server https://wuapi.example.internal:8443
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/deploymenttheory/go-sdk-windowsuup/catalog"
	"github.com/deploymenttheory/go-sdk-windowsuup/sdk"
)

func main() {
	server := flag.String("server", "https://localhost:8443", "winupdate server base URL")
	flag.Parse()

	client, err := newClient(*server)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Fprintf(os.Stderr, "Connecting to SSE feed at %s/v1/feed/stream ...\n", *server)
	fmt.Fprintln(os.Stderr, "Waiting for events (Ctrl-C to stop)...")
	fmt.Fprintln(os.Stderr, "")

	ch, err := client.Feed.Stream(ctx)
	if err != nil {
		log.Fatalf("connect to feed stream: %v", err)
	}

	for ev := range ch {
		ts := time.Now().UTC().Format("2006-01-02T15:04:05Z")
		fmt.Printf("[%s] event=%s\n", ts, ev.Event)

		// Try to pretty-print as a BuildEvent.
		var be catalog.BuildEvent
		if err := json.Unmarshal([]byte(ev.Data), &be); err == nil && be.Build.UUID != "" {
			fmt.Printf("  uuid:  %s\n", be.Build.UUID)
			fmt.Printf("  build: %s  arch=%s  ring=%s\n",
				be.Build.Build, be.Build.Arch, be.Build.Ring)
			if be.Build.Title != "" {
				fmt.Printf("  title: %s\n", be.Build.Title)
			}
		} else {
			// Unknown shape — print raw data.
			fmt.Printf("  data:  %s\n", ev.Data)
		}
		fmt.Println()
	}

	fmt.Fprintln(os.Stderr, "Feed stream closed.")
}

func newClient(server string) (*sdk.Client, error) {
	return sdk.NewClient(
		sdk.WithBaseURL(server),
		sdk.WithTimeout(0), // no timeout — stream runs indefinitely
		sdk.WithRetryCount(0),
	)
}
