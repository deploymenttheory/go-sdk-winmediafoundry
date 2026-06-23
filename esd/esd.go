// Package esd is a standalone client for Microsoft's Media Creation Tool ESD
// catalog. It fetches the signed products.cab, decompresses it (pure-Go LZX),
// parses the embedded products.xml, and returns the catalog of Windows
// installation ESDs — each with a direct, non-expiring CDN URL and SHA-1.
//
// It is structured like the windowsuup service client (transport, options,
// models, mocks) but is fully self-contained and independent of it.
//
// Quick start:
//
//	c, err := esd.NewClient()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	cat, _, err := c.Catalog(ctx, esdapi.WithProduct(esdapi.Windows11))
package esd

import (
	"fmt"

	esdapi "github.com/deploymenttheory/winmediafoundry/esd/api/esd"
	"github.com/deploymenttheory/winmediafoundry/esd/client"
	"go.uber.org/zap"
)

// Client is the entry point for the ESD catalog SDK. It is safe for concurrent
// use. The embedded *esdapi.Service promotes Catalog onto the Client, so callers
// can write c.Catalog(ctx, ...).
type Client struct {
	transport client.Client

	*esdapi.Service
}

// NewClient constructs an ESD catalog Client with the given options.
func NewClient(opts ...ClientOption) (*Client, error) {
	settings := &client.TransportSettings{}
	for _, o := range opts {
		if err := o(settings); err != nil {
			return nil, fmt.Errorf("esd.NewClient: apply option: %w", err)
		}
	}

	transport, err := client.NewTransport(settings)
	if err != nil {
		return nil, fmt.Errorf("esd.NewClient: %w", err)
	}

	return &Client{
		transport: transport,
		Service:   esdapi.New(transport),
	}, nil
}

// GetLogger returns the structured logger used by the client transport.
func (c *Client) GetLogger() *zap.Logger {
	return c.transport.GetLogger()
}
