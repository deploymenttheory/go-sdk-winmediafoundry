//go:build acceptance

// Acceptance tests make live SOAP calls to Microsoft's Windows Update service.
// They require outbound HTTPS access to fe3.delivery.mp.microsoft.com and
// fe3cr.delivery.mp.microsoft.com.
//
// Run with:
//
//	go test -tags=acceptance -timeout=120s -v ./windowsuup/acceptance/...
package acceptance_test

import (
	"context"
	"testing"
	"time"

	buildsapi "github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/api/builds"
	filesapi "github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/api/files"
	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup"
)

// newAcceptanceClient creates a Client for acceptance tests and fails the test
// immediately if the client cannot be constructed (e.g. TLS/connectivity failure).
func newAcceptanceClient(t *testing.T) *windowsuup.Client {
	t.Helper()
	c, err := windowsuup.NewClient()
	require.NoError(t, err, "NewClient: check TLS cert chain includes Microsoft Root CA 2011")
	return c
}

// ─── FetchBuilds ─────────────────────────────────────────────────────────────

func TestAcceptance_FetchBuilds_Retail(t *testing.T) {
	c := newAcceptanceClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	builds, _, err := c.Builds.FetchBuilds(ctx,
		buildsapi.WithArch(constants.ArchAMD64),
		buildsapi.WithRing(constants.RingRetail),
		buildsapi.WithSKU(constants.SKUPro),
	)
	require.NoError(t, err)
	require.NotEmpty(t, builds, "Retail amd64 must return at least one build")

	b := builds[0]
	assert.NotEmpty(t, b.UUID, "UUID must be non-empty")
	assert.NotEmpty(t, b.Build, "Build version must be non-empty (e.g. 10.0.26100.x)")
	assert.Equal(t, constants.ArchAMD64, b.Arch)
	assert.Equal(t, constants.RingRetail, b.Ring)
	assert.NotZero(t, b.Revision)

	t.Logf("Retail build: %s  %s  (UUID=%s  Rev=%d  Stable=%v)",
		b.Build, b.Title, b.UUID, b.Revision, b.IsStable)
}

func TestAcceptance_FetchBuilds_Experimental(t *testing.T) {
	c := newAcceptanceClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	builds, _, err := c.Builds.FetchBuilds(ctx,
		buildsapi.WithArch(constants.ArchAMD64),
		buildsapi.WithRing(constants.RingExperimental),
		buildsapi.WithSKU(constants.SKUPro),
	)
	require.NoError(t, err)
	require.NotEmpty(t, builds, "Experimental (Dev Insider) ring must return at least one build")

	b := builds[0]
	assert.NotEmpty(t, b.UUID)
	assert.True(t, b.IsInsider, "Experimental ring builds must be IsInsider=true")
	t.Logf("Experimental build: %s  %s  (UUID=%s)", b.Build, b.Title, b.UUID)
}

func TestAcceptance_FetchBuilds_ARM64(t *testing.T) {
	c := newAcceptanceClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	builds, _, err := c.Builds.FetchBuilds(ctx,
		buildsapi.WithArch(constants.ArchARM64),
		buildsapi.WithRing(constants.RingRetail),
		buildsapi.WithSKU(constants.SKUPro),
	)
	require.NoError(t, err)
	require.NotEmpty(t, builds)
	t.Logf("ARM64 Retail build: %s", builds[0].Build)
}

// ─── GetFiles ────────────────────────────────────────────────────────────────

func TestAcceptance_GetFiles_NoURLs(t *testing.T) {
	c := newAcceptanceClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	builds, _, err := c.Builds.FetchBuilds(ctx,
		buildsapi.WithArch(constants.ArchAMD64),
		buildsapi.WithRing(constants.RingRetail),
		buildsapi.WithSKU(constants.SKUPro),
	)
	require.NoError(t, err)
	require.NotEmpty(t, builds)

	files, _, err := c.Files.GetFiles(ctx, builds[0])
	require.NoError(t, err)
	require.NotEmpty(t, files, "GetFiles without CDN URLs must return file metadata")

	for _, f := range files {
		assert.NotEmpty(t, f.Name)
		assert.Empty(t, f.URL, "URL must be empty when WithCDNURLs not set")
	}
	t.Logf("GetFiles returned %d files for build %s", len(files), builds[0].Build)
}

func TestAcceptance_GetFiles_WithCDNURLs(t *testing.T) {
	c := newAcceptanceClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	builds, _, err := c.Builds.FetchBuilds(ctx,
		buildsapi.WithArch(constants.ArchAMD64),
		buildsapi.WithRing(constants.RingRetail),
		buildsapi.WithSKU(constants.SKUPro),
	)
	require.NoError(t, err)
	require.NotEmpty(t, builds)

	files, _, err := c.Files.GetFiles(ctx, builds[0], filesapi.WithCDNURLs())
	require.NoError(t, err)
	require.NotEmpty(t, files, "GetFiles with CDN URLs must return at least one file")

	for _, f := range files {
		assert.NotEmpty(t, f.Name)
		assert.NotEmpty(t, f.URL, "all files must have a CDN URL when WithCDNURLs is set")
		assert.False(t, f.ExpiresAt.IsZero(), "ExpiresAt must be populated from P1 parameter")
		assert.True(t, f.ExpiresAt.After(time.Now()), "CDN URLs must not be already expired")
	}
	t.Logf("GetFiles with URLs returned %d files; first: %s (expires %s)",
		len(files), files[0].Name, files[0].ExpiresAt.Format(time.RFC3339))
}

func TestAcceptance_GetFiles_LanguageFilter(t *testing.T) {
	c := newAcceptanceClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	builds, _, err := c.Builds.FetchBuilds(ctx,
		buildsapi.WithArch(constants.ArchAMD64),
		buildsapi.WithRing(constants.RingRetail),
		buildsapi.WithSKU(constants.SKUPro),
	)
	require.NoError(t, err)
	require.NotEmpty(t, builds)

	all, _, err := c.Files.GetFiles(ctx, builds[0], filesapi.WithCDNURLs())
	require.NoError(t, err)

	filtered, _, err := c.Files.GetFiles(ctx, builds[0], filesapi.WithCDNURLs(), filesapi.WithLanguage("en-us"))
	require.NoError(t, err)

	assert.LessOrEqual(t, len(filtered), len(all),
		"language-filtered set must be a subset of the full set")
	t.Logf("All: %d files; en-us filtered: %d files", len(all), len(filtered))
}

func TestAcceptance_GetFiles_ExtensionFilter(t *testing.T) {
	c := newAcceptanceClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	builds, _, err := c.Builds.FetchBuilds(ctx,
		buildsapi.WithArch(constants.ArchAMD64),
		buildsapi.WithRing(constants.RingRetail),
		buildsapi.WithSKU(constants.SKUPro),
	)
	require.NoError(t, err)
	require.NotEmpty(t, builds)

	files, _, err := c.Files.GetFiles(ctx, builds[0], filesapi.WithCDNURLs(), filesapi.WithExtension(".esd"))
	require.NoError(t, err)

	for _, f := range files {
		assert.Equal(t, "esd", f.FileType, "all returned files must be ESD")
	}
	t.Logf("ESD-only filter returned %d files", len(files))
}

// ─── Diff ────────────────────────────────────────────────────────────────────

func TestAcceptance_Diff_SameBuild(t *testing.T) {
	c := newAcceptanceClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	builds, _, err := c.Builds.FetchBuilds(ctx,
		buildsapi.WithArch(constants.ArchAMD64),
		buildsapi.WithRing(constants.RingRetail),
		buildsapi.WithSKU(constants.SKUPro),
	)
	require.NoError(t, err)
	require.NotEmpty(t, builds)

	// Diff a build against itself — everything must be unchanged.
	build := builds[0]
	d, _, err := c.Diff.Diff(ctx, build, build)
	require.NoError(t, err)

	assert.Empty(t, d.Added, "self-diff must have no added files")
	assert.Empty(t, d.Removed, "self-diff must have no removed files")
	assert.Empty(t, d.Changed, "self-diff must have no changed files")
	assert.Positive(t, d.Unchanged, "self-diff must report files as unchanged")
	t.Logf("Self-diff of %s: %d unchanged files", build.Build, d.Unchanged)
}

func TestAcceptance_Diff_TwoBuilds(t *testing.T) {
	c := newAcceptanceClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	retailBuilds, _, err := c.Builds.FetchBuilds(ctx,
		buildsapi.WithArch(constants.ArchAMD64),
		buildsapi.WithRing(constants.RingRetail),
		buildsapi.WithSKU(constants.SKUPro),
	)
	require.NoError(t, err)
	require.NotEmpty(t, retailBuilds)

	insiderBuilds, _, err := c.Builds.FetchBuilds(ctx,
		buildsapi.WithArch(constants.ArchAMD64),
		buildsapi.WithRing(constants.RingExperimental),
		buildsapi.WithSKU(constants.SKUPro),
	)
	require.NoError(t, err)
	require.NotEmpty(t, insiderBuilds)

	d, _, err := c.Diff.Diff(ctx, retailBuilds[0], insiderBuilds[0])
	require.NoError(t, err)

	total := len(d.Added) + len(d.Removed) + len(d.Changed) + d.Unchanged
	assert.Positive(t, total, "diff between Retail and Experimental must have some files")
	t.Logf("Retail %s vs Experimental %s: +%d -%d ~%d =%d",
		retailBuilds[0].Build, insiderBuilds[0].Build,
		len(d.Added), len(d.Removed), len(d.Changed), d.Unchanged)
}
