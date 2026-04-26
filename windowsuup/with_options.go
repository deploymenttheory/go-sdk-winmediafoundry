package windowsuup

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/client"
	"go.uber.org/zap"
)

// ClientOption configures a Client at construction time.
type ClientOption func(*client.TransportSettings)

// WithTimeout sets the per-SOAP-request HTTP timeout (default: 2 minutes).
// CDN download requests are exempt — they use a separate client with no read
// timeout to accommodate large file transfers.
func WithTimeout(d time.Duration) ClientOption {
	return func(s *client.TransportSettings) { s.Timeout = d }
}

// WithTLSConfig sets a custom TLS configuration for outbound SOAP connections.
// When nil the SDK uses its embedded Microsoft CA bundle plus the system cert
// pool.
func WithTLSConfig(cfg *tls.Config) ClientOption {
	return func(s *client.TransportSettings) { s.TLSConfig = cfg }
}

// WithHTTPClient replaces the underlying HTTP client entirely for SOAP calls.
// When set, WithTimeout and WithTLSConfig are ignored for SOAP requests.
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(s *client.TransportSettings) { s.HTTPClient = hc }
}

// WithLogger sets a custom zap logger (default: zap.NewProduction()).
func WithLogger(l *zap.Logger) ClientOption {
	return func(s *client.TransportSettings) { s.Logger = l }
}
