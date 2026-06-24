// Package client defines the transport interface and concrete implementation
// used by the softwaredownload service package. It mirrors the windowsuup/esd
// transport (retry, throttling, concurrency limiting, structured logging) but
// carries no Windows Update SOAP session state — the consumer download flow is
// plain HTTPS against Microsoft's public software-download endpoints.
package client

import (
	"context"

	"go.uber.org/zap"
)

// Client is the interface that the softwaredownload service depends on. It
// exposes generic HTTP request building — the only primitive needed to scrape
// the download pages, drive Microsoft's download-connector API, and stream the
// resulting ISO — without coupling the service to a concrete transport.
type Client interface {
	// NewRequest returns a RequestBuilder for this transport. The service layer
	// uses it to construct the full request — headers, query params, body —
	// before calling Get/Post to execute it. Retry, concurrency limiting, and
	// throttling are applied by the transport.
	NewRequest(ctx context.Context) *RequestBuilder

	// GetLogger returns the structured logger shared across the SDK.
	GetLogger() *zap.Logger
}
