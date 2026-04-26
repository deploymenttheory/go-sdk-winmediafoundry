// Package constants defines API endpoint paths and default values used across
// the SDK sub-packages.
package constants

const (
	// API endpoint paths.
	EndpointHealth     = "/healthz"
	EndpointReady      = "/readyz"
	EndpointBuilds     = "/v1/builds"
	EndpointUpdates    = "/v1/updates/fetch"
	EndpointDiff       = "/v1/diff"
	EndpointFeed       = "/v1/feed"
	EndpointFeedStream = "/v1/feed/stream"

	// DefaultUserAgent is the User-Agent header sent with every request.
	DefaultUserAgent = "go-sdk-windowsuup/1.0"

	// Content types.
	ContentTypeJSON = "application/json"
	ContentTypeSSE  = "text/event-stream"
)
