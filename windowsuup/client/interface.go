// Package client defines the transport interface and concrete implementation
// used by all Windows Update SDK service packages.
package client

import (
	"context"

	"github.com/deploymenttheory/go-sdk-windowsuup/internal/wuproto"
	"go.uber.org/zap"
	"resty.dev/v3"
)

// Client is the interface that all API service packages depend on.
// It exposes the underlying SOAP operations and supporting utilities
// without coupling services to a concrete transport implementation.
type Client interface {
	// FetchUpdates calls the SyncUpdates SOAP endpoint to discover available builds.
	FetchUpdates(ctx context.Context, req wuproto.FetchRequest) ([]wuproto.UpdateResult, *resty.Response, error)

	// GetFileURLs calls GetExtendedUpdateInfo2 to resolve live CDN download URLs
	// for the files associated with a specific build revision.
	GetFileURLs(ctx context.Context, req wuproto.FileURLRequest) ([]wuproto.FileURL, *resty.Response, error)

	// GetDownloadClient returns the resty client used for CDN file downloads.
	// This client has no read timeout and is configured with SetDoNotParseResponse
	// to support streaming of large files.
	GetDownloadClient() *resty.Client

	// GetLogger returns the structured logger shared across the SDK.
	GetLogger() *zap.Logger
}
