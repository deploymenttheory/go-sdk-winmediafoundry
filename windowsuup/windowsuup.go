// Package windowsuup provides a Go client for Microsoft's Windows Update SOAP
// API. It discovers Windows builds, resolves CDN download URLs, downloads
// ESD/CAB files, and compares build file sets — all via direct SOAP calls to
// Microsoft, with no intermediary service required.
//
// Quick start:
//
//	c, err := windowsuup.NewClient()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	builds, err := c.Builds.FetchBuilds(ctx,
//	    builds.WithArch(constants.ArchAMD64),
//	    builds.WithRing(constants.RingRetail),
//	)
//
//	files, err := c.Files.GetFiles(ctx, builds[0],
//	    files.WithLanguage("en-us"),
//	    files.WithEdition(constants.EditionProfessional),
//	    files.WithCDNURLs(),
//	)
//
//	err = c.Download.DownloadFiles(ctx, files, "/tmp/win11", 4)
package windowsuup

import (
	"fmt"

	buildsapi "github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/api/builds"
	diffapi "github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/api/diff"
	downloadapi "github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/api/download"
	esdapi "github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/api/esd"
	filesapi "github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/api/files"
	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/client"
	"go.uber.org/zap"
)

// Client is the entry point for the Windows Update SDK.
// It is safe for concurrent use.
type Client struct {
	transport client.Client

	// Builds exposes Windows Update build discovery operations.
	Builds *buildsapi.Service

	// Files exposes file resolution operations for a given build.
	Files *filesapi.Service

	// Download exposes CDN file download operations.
	Download *downloadapi.Service

	// Diff exposes build file-set comparison operations.
	Diff *diffapi.Service

	// ESD exposes Windows installation ESD catalog resolution (Media Creation
	// Tool products.cab) — how a full, bootable install.esd is obtained.
	ESD *esdapi.Service
}

// NewClient constructs a Client with the given options.
func NewClient(opts ...ClientOption) (*Client, error) {
	settings := &client.TransportSettings{}
	for _, o := range opts {
		if err := o(settings); err != nil {
			return nil, fmt.Errorf("windowsuup.NewClient: apply option: %w", err)
		}
	}

	transport, err := client.NewTransport(settings)
	if err != nil {
		return nil, fmt.Errorf("windowsuup.NewClient: %w", err)
	}

	return &Client{
		transport: transport,
		Builds:    buildsapi.New(transport),
		Files:     filesapi.New(transport),
		Download:  downloadapi.New(transport),
		Diff:      diffapi.New(transport),
		ESD:       esdapi.New(transport),
	}, nil
}

// GetLogger returns the structured logger used by the SDK transport.
func (c *Client) GetLogger() *zap.Logger {
	return c.transport.GetLogger()
}
