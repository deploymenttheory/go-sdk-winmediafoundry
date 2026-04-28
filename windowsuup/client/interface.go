// Package client defines the transport interface and concrete implementation
// used by all Windows Update SDK service packages.
package client

import (
	"context"

	"go.uber.org/zap"
)

// Client is the interface that all API service packages depend on.
// It exposes generic HTTP request building plus Windows Update session cookie
// management — the two primitives needed to execute SOAP calls and CDN
// downloads without coupling services to a concrete transport implementation.
type Client interface {
	// NewRequest returns a RequestBuilder for this transport. The service layer
	// uses it to construct the full request — headers, body, query params —
	// before calling Get/Post/Delete to execute it. Retry, concurrency limiting,
	// and throttling are applied by the transport.
	NewRequest(ctx context.Context) *RequestBuilder

	// GetLogger returns the structured logger shared across the SDK.
	GetLogger() *zap.Logger

	// AcquireWUCookie returns the current Windows Update session cookie values
	// needed to construct SOAP request bodies. The cookie is fetched lazily and
	// cached for ~14 minutes; subsequent calls return the cached value unless it
	// has expired.
	//
	// encryptedData and expiration are embedded in SyncUpdates request bodies.
	// deviceToken is the device ticket used across all SOAP calls.
	AcquireWUCookie(ctx context.Context) (encryptedData, expiration, deviceToken string, err error)

	// InvalidateWUCookie clears the cached WU session cookie so the next
	// AcquireWUCookie call triggers a fresh GetCookie SOAP request.
	// Call this after receiving an HTTP 500 response that indicates a stale
	// or expired cookie (see soap.IsCookieError).
	InvalidateWUCookie()
}
