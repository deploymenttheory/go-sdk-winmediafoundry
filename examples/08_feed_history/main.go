// 08_feed_history demonstrates GET /v1/feed.
//
// It retrieves paginated entries from the build change-feed — a record of
// every build discovery and update event. Useful for auditing what changed
// and when, or for replaying events that were missed while the SSE stream
// was not connected.
//
// Use --event-type to filter to a specific event class:
//   - "build_discovered" — first time a build UUID was seen
//   - "build_updated"    — a build's metadata was refreshed
//
// Usage:
//
//	go run ./examples/08_feed_history
//	go run ./examples/08_feed_history --event-type build_discovered --limit 50
//	go run ./examples/08_feed_history --since 2026-01-01T00:00:00Z
//
//	go run ./examples/08_feed_history \
//	  --server https://wuapi.example.internal:8443
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/deploymenttheory/go-sdk-windowsuup/catalog"
	"github.com/deploymenttheory/go-sdk-windowsuup/sdk"
)

func main() {
	server := flag.String("server", "https://localhost:8443", "winupdate server base URL")
	eventType := flag.String("event-type", "", "filter by event type: build_discovered, build_updated")
	since := flag.String("since", "", "return events after this RFC3339 timestamp, e.g. 2026-01-01T00:00:00Z")
	limit := flag.Int("limit", 20, "maximum number of entries to return")
	offset := flag.Int("offset", 0, "pagination offset")
	flag.Parse()

	client, err := newClient(*server)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	q := catalog.FeedQuery{
		EventType: *eventType,
		Limit:     *limit,
		Offset:    *offset,
	}
	if *since != "" {
		t, err := time.Parse(time.RFC3339, *since)
		if err != nil {
			log.Fatalf("parse --since: %v (expected RFC3339 format, e.g. 2026-01-01T00:00:00Z)", err)
		}
		q.Since = t
	}

	entries, total, err := client.Feed.List(ctx, q)
	if err != nil {
		log.Fatalf("list feed: %v", err)
	}

	if len(entries) == 0 {
		fmt.Println("No feed entries found.")
		fmt.Println("Tip: run 01_fetch_updates to discover builds and populate the feed.")
		return
	}

	fmt.Printf("Showing %d of %d total event(s) (offset=%d)\n\n", len(entries), total, *offset)
	fmt.Printf("%-26s  %-20s  %-14s  %-6s  %-14s  %s\n",
		"Occurred At", "Event", "Build", "Arch", "Ring", "Title")
	fmt.Printf("%-26s  %-20s  %-14s  %-6s  %-14s  %s\n",
		"--------------------------", "--------------------", "--------------",
		"------", "--------------", "-----")

	for _, e := range entries {
		fmt.Printf("%-26s  %-20s  %-14s  %-6s  %-14s  %s\n",
			e.OccurredAt.UTC().Format("2006-01-02T15:04:05Z"),
			e.EventType,
			e.BuildNumber,
			e.Arch,
			e.Ring,
			e.BuildTitle,
		)
	}

	if int64(*offset+*limit) < total {
		fmt.Printf("\nMore events available — use --offset %d to see the next page.\n", *offset+*limit)
	}
}

func newClient(server string) (*sdk.Client, error) {
	return sdk.NewClient(
		sdk.WithBaseURL(server),
		sdk.WithTimeout(30*time.Second),
		sdk.WithRetryCount(2),
	)
}
