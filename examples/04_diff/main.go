// Example 04_diff: compares the file sets of the two most recent amd64 Retail
// builds and prints what was added, removed, and changed between them.
//
// If only one build is available the diff is against itself (all unchanged).
//
// Run:
//
//	go run ./examples/04_diff
package main

import (
	"context"
	"fmt"
	"log"

	buildsapi "github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/api/builds"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/constants"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup"
)

func main() {
	ctx := context.Background()

	client, err := windowsuup.NewClient()
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}

	builds, _, err := client.Builds.FetchBuilds(ctx,
		buildsapi.WithArch(constants.ArchAMD64),
		buildsapi.WithRing(constants.RingRetail),
		buildsapi.WithSKU(constants.SKUPro),
	)
	if err != nil {
		log.Fatalf("FetchBuilds: %v", err)
	}
	if len(builds) == 0 {
		log.Fatal("no builds found")
	}

	// Use the first two builds; fall back to comparing the same build twice.
	buildA := builds[0]
	buildB := builds[0]
	if len(builds) >= 2 {
		buildB = builds[1]
	}

	fmt.Printf("Base:   %s  (%s)\n", buildA.Build, buildA.UUID)
	fmt.Printf("Target: %s  (%s)\n\n", buildB.Build, buildB.UUID)

	d, _, err := client.Diff.Diff(ctx, buildA, buildB)
	if err != nil {
		log.Fatalf("Diff: %v", err)
	}

	fmt.Printf("Added:     %d\n", len(d.Added))
	fmt.Printf("Removed:   %d\n", len(d.Removed))
	fmt.Printf("Changed:   %d\n", len(d.Changed))
	fmt.Printf("Unchanged: %d\n\n", d.Unchanged)

	if len(d.Added) > 0 {
		fmt.Println("--- Added ---")
		for _, f := range d.Added {
			fmt.Printf("  + %s  (%d bytes)\n", f.Name, f.SizeBytes)
		}
	}
	if len(d.Removed) > 0 {
		fmt.Println("--- Removed ---")
		for _, f := range d.Removed {
			fmt.Printf("  - %s  (%d bytes)\n", f.Name, f.SizeBytes)
		}
	}
	if len(d.Changed) > 0 {
		fmt.Println("--- Changed ---")
		for _, fd := range d.Changed {
			fmt.Printf("  ~ %s  (%d → %d bytes)\n",
				fd.Name, fd.BaseFile.SizeBytes, fd.TargetFile.SizeBytes)
		}
	}
}
