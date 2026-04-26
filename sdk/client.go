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
	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/builds"
	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/diff"
	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/feed"
	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/files"
	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/transport"
	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/updates"
	"go.uber.org/zap"
)

// Client is the entry point for the Windows Update Metadata API SDK.
// It is safe for concurrent use.
type Client struct {
	t      *transport.Transport
	logger *zap.Logger

	Builds  *builds.Builds
	Files   *files.Files
	Updates *updates.Updates
	Diff    *diff.Diff
	Feed    *feed.Feed
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

	return &Client{
		t:       t,
		logger:  logger,
		Builds:  builds.New(t),
		Files:   files.New(t),
		Updates: updates.New(t),
		Diff:    diff.New(t),
		Feed:    feed.New(t),
	}, nil
}
