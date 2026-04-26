// Package sdk provides a Go client for the Windows Update Metadata HTTP API.
//
// Create a client with NewClient, then use the service fields (Builds, Files,
// Updates, Diff, Feed) to interact with the API.
//
// Example:
//
//	client, err := sdk.NewClient(
//	    sdk.WithBaseURL("https://wuapi.example.internal:8443"),
//	    sdk.WithMTLS("client.crt", "client.key", "ca.crt"),
//	)
//	builds, _, err := client.Builds.List(ctx, catalog.BuildQuery{StableOnly: true})
package sdk

import (
	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/transport"
	"go.uber.org/zap"
)

// Client is the entry point for the Windows Update Metadata API SDK.
// It is safe for concurrent use.
type Client struct {
	t      *transport.Transport
	logger *zap.Logger

	Builds  *BuildsService
	Files   *FilesService
	Updates *UpdatesService
	Diff    *DiffService
	Feed    *FeedService
}

// NewClient creates a Client with the given options applied to the default settings.
func NewClient(opts ...ClientOption) (*Client, error) {
	settings := transport.DefaultSettings()
	for _, o := range opts {
		o(settings)
	}

	logger, err := zap.NewProduction()
	if err != nil {
		return nil, err
	}

	t, err := transport.New(settings, logger)
	if err != nil {
		return nil, err
	}

	c := &Client{t: t, logger: logger}
	c.Builds = &BuildsService{t: t}
	c.Files = &FilesService{t: t}
	c.Updates = &UpdatesService{t: t}
	c.Diff = &DiffService{t: t}
	c.Feed = &FeedService{t: t}
	return c, nil
}
