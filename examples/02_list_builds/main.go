// 02_list_builds demonstrates GET /v1/builds.
//
// It queries the local build catalog with optional filters for architecture,
// ring, text search, and stable-only. Results are sorted by discovery time
// (newest first) and displayed as a formatted table with totals.
//
// Usage:
//
//	go run ./examples/02_list_builds
//	go run ./examples/02_list_builds --arch amd64 --ring Retail
//	go run ./examples/02_list_builds --search "24H2" --stable
//
//	go run ./examples/02_list_builds \
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
	search := flag.String("search", "", "substring filter on build title or number")
	arch := flag.String("arch", "", "filter by architecture: amd64, arm64, x86")
	ring := flag.String("ring", "", "filter by ring: Retail, ReleasePreview, Beta, Dev, Canary")
	stable := flag.Bool("stable", false, "return stable (non-Insider) builds only")
	limit := flag.Int("limit", 20, "maximum number of results to return")
	offset := flag.Int("offset", 0, "pagination offset")
	flag.Parse()

	client, err := newClient(*server)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	builds, total, err := client.Builds.List(ctx, catalog.BuildQuery{
		Search:     *search,
		Arch:       *arch,
		Ring:       *ring,
		StableOnly: *stable,
		Limit:      *limit,
		Offset:     *offset,
		OrderBy:    "discovered_at",
		Desc:       true,
	})
	if err != nil {
		log.Fatalf("list builds: %v", err)
	}

	if len(builds) == 0 {
		fmt.Println("No builds found — run example 01_fetch_updates first to populate the catalog.")
		return
	}

	fmt.Printf("Showing %d of %d total builds (offset=%d)\n\n", len(builds), total, *offset)
	fmt.Printf("%-38s  %-14s  %-6s  %-14s  %-6s  %s\n",
		"UUID", "Build", "Arch", "Ring", "Stable", "Title")
	fmt.Printf("%-38s  %-14s  %-6s  %-14s  %-6s  %s\n",
		"--------------------------------------", "--------------", "------",
		"--------------", "------", "-----")

	for _, b := range builds {
		stable := "no"
		if b.IsStable {
			stable = "yes"
		}
		title := b.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		fmt.Printf("%-38s  %-14s  %-6s  %-14s  %-6s  %s\n",
			b.UUID, b.Build, b.Arch, b.Ring, stable, title)
	}

	if int64(*offset+*limit) < total {
		fmt.Printf("\nMore results available — use --offset %d to see the next page.\n", *offset+*limit)
	}
}

func newClient(server string) (*sdk.Client, error) {
	return sdk.NewClient(
		sdk.WithBaseURL(server),
		sdk.WithTimeout(30*time.Second),
		sdk.WithRetryCount(2),
	)
}
