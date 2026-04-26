// 07_diff_builds demonstrates GET /v1/diff.
//
// It compares the file sets of two builds and reports what changed: which files
// were added, removed, or modified (by SHA1 or size). Useful for understanding
// what changed between two cumulative updates or feature releases.
//
// Usage:
//
//	go run ./examples/07_diff_builds \
//	  --base  <uuid-of-older-build> \
//	  --target <uuid-of-newer-build>
//
//	go run ./examples/07_diff_builds \
//	  --server https://wuapi.example.internal:8443 \
//	  --base  <uuid-of-older-build> \
//	  --target <uuid-of-newer-build>
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
	base := flag.String("base", "", "UUID of the older (base) build (required)")
	target := flag.String("target", "", "UUID of the newer (target) build (required)")
	verbose := flag.Bool("verbose", false, "print all added/removed files, not just changed")
	flag.Parse()

	if *base == "" || *target == "" {
		fmt.Fprintln(os.Stderr, "error: --base and --target are required")
		flag.Usage()
		os.Exit(1)
	}

	client, err := newClient(*server)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Printf("Comparing builds:\n  base:   %s\n  target: %s\n\n", *base, *target)

	diff, err := client.Diff.Compare(ctx, *base, *target)
	if err != nil {
		log.Fatalf("diff builds: %v", err)
	}

	fmt.Printf("base build:   %s\n", diff.BaseBuild)
	fmt.Printf("target build: %s\n", diff.TargetBuild)
	fmt.Printf("generated:    %s\n\n", diff.GeneratedAt.Format(time.RFC3339))

	fmt.Printf("Summary:\n")
	fmt.Printf("  added:     %d\n", len(diff.Added))
	fmt.Printf("  removed:   %d\n", len(diff.Removed))
	fmt.Printf("  changed:   %d\n", len(diff.Changed))
	fmt.Printf("  unchanged: %d\n\n", diff.Unchanged)

	if len(diff.Changed) > 0 {
		fmt.Println("=== Changed files ===")
		fmt.Printf("%-55s  %12s  %12s\n", "Name", "Old size", "New size")
		fmt.Printf("%-55s  %12s  %12s\n",
			"-------------------------------------------------------", "------------", "------------")
		for _, e := range diff.Changed {
			oldSize := int64(0)
			newSize := int64(0)
			if e.BaseFile != nil {
				oldSize = e.BaseFile.SizeBytes
			}
			if e.TargetFile != nil {
				newSize = e.TargetFile.SizeBytes
			}
			delta := ""
			if newSize > oldSize {
				delta = fmt.Sprintf(" (+%s)", formatBytes(newSize-oldSize))
			} else if newSize < oldSize {
				delta = fmt.Sprintf(" (-%s)", formatBytes(oldSize-newSize))
			}
			fmt.Printf("%-55s  %12s  %12s%s\n",
				e.Name, formatBytes(oldSize), formatBytes(newSize), delta)
		}
	}

	if *verbose {
		if len(diff.Added) > 0 {
			fmt.Println("\n=== Added files ===")
			for _, e := range diff.Added {
				size := int64(0)
				if e.TargetFile != nil {
					size = e.TargetFile.SizeBytes
				}
				fmt.Printf("  + %-55s  %s\n", e.Name, formatBytes(size))
			}
		}
		if len(diff.Removed) > 0 {
			fmt.Println("\n=== Removed files ===")
			for _, e := range diff.Removed {
				size := int64(0)
				if e.BaseFile != nil {
					size = e.BaseFile.SizeBytes
				}
				fmt.Printf("  - %-55s  %s\n", e.Name, formatBytes(size))
			}
		}
	} else if len(diff.Added)+len(diff.Removed) > 0 {
		fmt.Printf("\nRun with --verbose to see the full added/removed file lists.\n")
	}
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
		sdk.WithTimeout(30*time.Second),
		sdk.WithRetryCount(2),
	)
}
