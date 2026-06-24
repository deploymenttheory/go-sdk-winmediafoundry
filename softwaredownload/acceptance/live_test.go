// Package acceptance holds live, network-dependent tests for the
// softwaredownload service. They are skipped unless SWDL_LIVE=1 is set, since
// they hit Microsoft's real software-download endpoints.
package acceptance

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload"
	sdapi "github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/api/softwaredownload"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/constants"
	"go.uber.org/zap"
)

func liveClient(t *testing.T) *softwaredownload.Client {
	t.Helper()
	if os.Getenv("SWDL_LIVE") != "1" {
		t.Skip("set SWDL_LIVE=1 to run live software-download tests")
	}
	c, err := softwaredownload.NewClient(softwaredownload.WithLogger(zap.NewNop()))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

// TestLiveScrape proves Get/List return the real product editions, including the
// ARM64 multi-edition ISO (edition id 3324 at time of writing).
func TestLiveScrape(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	products, _, err := c.List(ctx, sdapi.WithArch(constants.ArchARM64))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(products) == 0 {
		t.Fatal("List returned no ARM64 products")
	}
	for _, p := range products {
		t.Logf("edition %s: %q (%s)", p.EditionID, p.Name, p.Arch)
		if p.Arch != constants.ArchARM64 {
			t.Errorf("expected ARM64 product, got %s for %q", p.Arch, p.Name)
		}
	}
}

// TestLiveResolveARM64 proves the full session → SKU → links flow yields a signed
// ARM64 ISO URL (without downloading the multi-GB file).
func TestLiveResolveARM64(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	link, _, err := c.GetByName(ctx, "Arm64", sdapi.WithArch(constants.ArchARM64))
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}

	t.Logf("resolved: file=%s arch=%s expires=%s\n  url=%s",
		link.FileName, link.Arch, link.ExpiresAt.Format(time.RFC3339), link.URL)

	if link.Arch != constants.ArchARM64 {
		t.Errorf("resolved arch = %s, want ARM64", link.Arch)
	}
	if !strings.Contains(strings.ToLower(link.FileName), "arm64") {
		t.Errorf("resolved file %q does not look like ARM64 media", link.FileName)
	}
	if !strings.HasPrefix(link.URL, "https://") || !strings.Contains(link.URL, ".microsoft.com") {
		t.Errorf("resolved URL is not a Microsoft https link: %q", link.URL)
	}
}
