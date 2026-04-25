// sdk_basic demonstrates basic usage of the go-sdk-uupdump SDK client.
//
// It connects to a local winupdate server (plain HTTP, no mTLS) and:
//   - Lists the 5 most-recent stable builds from the catalog.
//   - Fetches file metadata for the first build.
//   - Compares the two most-recent builds.
//
// Usage:
//
//	go run ./examples/sdk_basic --server http://localhost:8080
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/deploymenttheory/go-sdk-uupdump/catalog"
	"github.com/deploymenttheory/go-sdk-uupdump/sdk"
)

func main() {
	server := flag.String("server", "http://localhost:8080", "winupdate server base URL")
	flag.Parse()

	client, err := sdk.NewClient(
		sdk.WithBaseURL(*server),
		sdk.WithTimeout(30*time.Second),
		sdk.WithRetryCount(2),
		sdk.WithInsecureSkipVerify(), // plain HTTP demo — not for production
	)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	ctx := context.Background()
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	// ── List stable builds ───────────────────────────────────────────────────
	fmt.Println("=== Recent stable builds ===")
	builds, total, err := client.Builds.List(ctx, catalog.BuildQuery{
		StableOnly: true,
		Limit:      5,
		OrderBy:    "discovered_at",
		Desc:       true,
	})
	if err != nil {
		log.Fatalf("list builds: %v", err)
	}
	fmt.Printf("total: %d\n", total)
	if err := enc.Encode(builds); err != nil {
		log.Fatal(err)
	}

	if len(builds) == 0 {
		fmt.Println("no builds in catalog — run `winupdate fetch` first")
		return
	}

	// ── List files for the first build ──────────────────────────────────────
	first := builds[0]
	fmt.Printf("\n=== Files for build %s (%s) ===\n", first.Build, first.UUID)
	files, err := client.Files.QueryFiles(first.UUID).
		ByExtension(".esd").
		LargerThan(100 * 1024 * 1024). // > 100 MB
		Execute(ctx)
	if err != nil {
		log.Fatalf("list files: %v", err)
	}
	fmt.Printf("%d ESD files > 100 MB:\n", len(files))
	if err := enc.Encode(files); err != nil {
		log.Fatal(err)
	}

	// ── Diff two builds ──────────────────────────────────────────────────────
	if len(builds) >= 2 {
		second := builds[1]
		fmt.Printf("\n=== Diff %s → %s ===\n", second.Build, first.Build)
		diff, err := client.Diff.Compare(ctx, second.UUID, first.UUID)
		if err != nil {
			log.Fatalf("diff: %v", err)
		}
		fmt.Printf("added=%d removed=%d changed=%d unchanged=%d\n",
			len(diff.Added), len(diff.Removed), len(diff.Changed), diff.Unchanged)
	}
}
