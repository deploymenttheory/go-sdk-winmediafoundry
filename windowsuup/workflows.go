package windowsuup

import (
	"context"
	"fmt"

	buildsapi "github.com/deploymenttheory/winmediafoundry/windowsuup/api/builds"
	filesapi "github.com/deploymenttheory/winmediafoundry/windowsuup/api/files"
	"github.com/deploymenttheory/winmediafoundry/windowsuup/shared/models"
)

// DownloadBuild resolves CDN download URLs for build's files, applies the
// given filters, and downloads matching files concurrently to destDir.
//
// It is equivalent to calling:
//
//	files, _, err := client.Files.GetFiles(ctx, build, filesapi.WithCDNURLs(), ...)
//	err = client.Download.DownloadFiles(ctx, files, destDir, concurrency)
//
// All DownloadBuildOption filters are applied to the GetFiles call. Files are
// written atomically and files already present at the correct size are skipped.
func (c *Client) DownloadBuild(ctx context.Context, build models.Build, destDir string, opts ...DownloadBuildOption) error {
	cfg := &downloadBuildConfig{concurrency: 4}
	for _, o := range opts {
		o(cfg)
	}

	fileOpts := []filesapi.FileOption{filesapi.WithCDNURLs()}
	if cfg.language != "" {
		fileOpts = append(fileOpts, filesapi.WithLanguage(cfg.language))
	}
	if cfg.edition != "" {
		fileOpts = append(fileOpts, filesapi.WithEdition(cfg.edition))
	}
	if cfg.extension != "" {
		fileOpts = append(fileOpts, filesapi.WithExtension(cfg.extension))
	}

	files, _, err := c.Files.GetFiles(ctx, build, fileOpts...)
	if err != nil {
		return fmt.Errorf("DownloadBuild: get files for %s: %w", build.UUID, err)
	}

	if err := c.Download.DownloadFiles(ctx, files, destDir, cfg.concurrency); err != nil {
		return fmt.Errorf("DownloadBuild: %w", err)
	}
	return nil
}

// FetchLatestAndDownload discovers the most recently offered build matching
// fetchOpts, then calls DownloadBuild with downloadOpts.
//
// fetchOpts are forwarded unchanged to Builds.FetchBuilds; the first build
// in the result is used. Returns an error if no builds are found.
//
// Example — download the latest Windows 11 Pro en-us ESD files:
//
//	err := client.FetchLatestAndDownload(ctx, "./downloads",
//	    []buildsapi.FetchOption{
//	        buildsapi.WithArch(constants.ArchAMD64),
//	        buildsapi.WithRing(constants.RingRetail),
//	        buildsapi.WithSKU(constants.SKUPro),
//	    },
//	    []DownloadBuildOption{
//	        windowsuup.WithDownloadLanguage("en-us"),
//	        windowsuup.WithDownloadEdition(constants.EditionProfessional),
//	        windowsuup.WithDownloadExtension(".esd"),
//	    },
//	)
func (c *Client) FetchLatestAndDownload(
	ctx context.Context,
	destDir string,
	fetchOpts []buildsapi.FetchOption,
	downloadOpts []DownloadBuildOption,
) error {
	builds, _, err := c.Builds.FetchBuilds(ctx, fetchOpts...)
	if err != nil {
		return fmt.Errorf("FetchLatestAndDownload: fetch builds: %w", err)
	}
	if len(builds) == 0 {
		return fmt.Errorf("FetchLatestAndDownload: no builds found for the given options")
	}

	if err := c.DownloadBuild(ctx, builds[0], destDir, downloadOpts...); err != nil {
		return fmt.Errorf("FetchLatestAndDownload: %w", err)
	}
	return nil
}
