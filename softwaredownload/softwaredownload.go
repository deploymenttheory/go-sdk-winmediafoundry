// Package softwaredownload is a standalone client for Microsoft's consumer
// software-download site. It scrapes the Windows 11 ISO download pages, drives
// Microsoft's download-connector API to resolve a signed, time-limited ISO
// download link for a chosen edition and language, and streams the ISO to disk.
//
// It is structured like the windowsuup service client (transport, options,
// models, mocks) but is fully self-contained and independent of it.
//
// Quick start:
//
//	c, err := softwaredownload.NewClient()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Scrape the available product editions.
//	cat, _, err := c.Get(ctx, softwaredownload.WithArch(constants.ArchARM64))
//
//	// Resolve and download an ARM64 ISO by name.
//	link, _, err := c.GetByName(ctx, "Arm64",
//	    softwaredownload.WithDownloadDir("/tmp/win"),
//	    softwaredownload.WithProgress(nil),
//	)
package softwaredownload

import (
	"fmt"

	sdapi "github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/api/softwaredownload"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/client"
	"go.uber.org/zap"
)

// Client is the entry point for the software-download SDK. It is safe for
// concurrent use. The embedded *sdapi.Service promotes Get/List/GetByID/
// GetByName/Download onto the Client, so callers can write c.Get(ctx, ...).
type Client struct {
	transport client.Client

	*sdapi.Service
}

// NewClient constructs a software-download Client with the given options.
func NewClient(opts ...ClientOption) (*Client, error) {
	settings := &client.TransportSettings{}
	for _, o := range opts {
		if err := o(settings); err != nil {
			return nil, fmt.Errorf("softwaredownload.NewClient: apply option: %w", err)
		}
	}

	transport, err := client.NewTransport(settings)
	if err != nil {
		return nil, fmt.Errorf("softwaredownload.NewClient: %w", err)
	}

	return &Client{
		transport: transport,
		Service:   sdapi.New(transport),
	}, nil
}

// GetLogger returns the structured logger used by the client transport.
func (c *Client) GetLogger() *zap.Logger {
	return c.transport.GetLogger()
}
