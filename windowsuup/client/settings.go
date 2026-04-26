package client

import (
	"crypto/tls"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// TransportSettings holds all configuration applied when constructing a Transport.
// It is populated by the ClientOption functions defined in with_options.go.
type TransportSettings struct {
	// Timeout is the per-SOAP-request HTTP timeout. Default: 2 minutes.
	// CDN download requests are exempt — they use a separate HTTP client with no
	// read timeout to accommodate large file transfers.
	Timeout time.Duration

	// TLSConfig is an optional custom TLS configuration for outbound connections.
	// When nil the transport uses the embedded Microsoft CA bundle (see certs.go
	// in the internal SOAP package) plus the system cert pool.
	TLSConfig *tls.Config

	// HTTPClient replaces the underlying HTTP client entirely for SOAP calls.
	// When set, Timeout and TLSConfig are ignored for SOAP requests.
	HTTPClient *http.Client

	// Logger is the zap logger used throughout the SDK.
	// Defaults to zap.NewProduction() when nil.
	Logger *zap.Logger
}

// DefaultTransportSettings returns a TransportSettings populated with
// production-safe defaults.
func DefaultTransportSettings() *TransportSettings {
	return &TransportSettings{
		Timeout: 2 * time.Minute,
	}
}
