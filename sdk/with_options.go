package sdk

import (
	"time"

	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/transport"
	"go.uber.org/zap"
)

// ClientOption is a functional option for configuring the SDK Client.
type ClientOption func(*transport.Settings)

// WithBaseURL overrides the API server base URL.
func WithBaseURL(url string) ClientOption {
	return func(s *transport.Settings) { s.BaseURL = url }
}

// WithTimeout sets the per-request HTTP timeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(s *transport.Settings) { s.Timeout = d }
}

// WithRetryCount sets the number of retry attempts.
func WithRetryCount(n int) ClientOption {
	return func(s *transport.Settings) { s.RetryCount = n }
}

// WithRetryWaitTime sets the minimum wait between retries.
func WithRetryWaitTime(d time.Duration) ClientOption {
	return func(s *transport.Settings) { s.RetryWaitTime = d }
}

// WithRetryMaxWaitTime sets the maximum wait between retries.
func WithRetryMaxWaitTime(d time.Duration) ClientOption {
	return func(s *transport.Settings) { s.RetryMaxWaitTime = d }
}

// WithTotalRetryDuration sets the wall-clock budget for a request including retries.
func WithTotalRetryDuration(d time.Duration) ClientOption {
	return func(s *transport.Settings) { s.TotalRetryDuration = d }
}

// WithMaxConcurrentRequests caps the number of in-flight requests.
func WithMaxConcurrentRequests(n int) ClientOption {
	return func(s *transport.Settings) { s.MaxConcurrentRequests = n }
}

// WithMandatoryRequestDelay introduces a fixed pause after every successful request.
func WithMandatoryRequestDelay(d time.Duration) ClientOption {
	return func(s *transport.Settings) { s.MandatoryRequestDelay = d }
}

// WithUserAgent sets a custom User-Agent header.
func WithUserAgent(ua string) ClientOption {
	return func(s *transport.Settings) { s.UserAgent = ua }
}

// WithGlobalHeader adds a header to every request.
func WithGlobalHeader(key, value string) ClientOption {
	return func(s *transport.Settings) {
		if s.GlobalHeaders == nil {
			s.GlobalHeaders = make(map[string]string)
		}
		s.GlobalHeaders[key] = value
	}
}

// WithGlobalHeaders adds multiple headers to every request.
func WithGlobalHeaders(headers map[string]string) ClientOption {
	return func(s *transport.Settings) {
		if s.GlobalHeaders == nil {
			s.GlobalHeaders = make(map[string]string)
		}
		for k, v := range headers {
			s.GlobalHeaders[k] = v
		}
	}
}

// WithProxy routes requests through the given HTTP proxy URL.
func WithProxy(url string) ClientOption {
	return func(s *transport.Settings) { s.ProxyURL = url }
}

// WithInsecureSkipVerify disables TLS certificate verification. Testing only.
func WithInsecureSkipVerify() ClientOption {
	return func(s *transport.Settings) { s.InsecureSkipVerify = true }
}

// WithMTLS configures mutual TLS with the given client certificate, key,
// and CA certificate paths.
func WithMTLS(certFile, keyFile, caCertFile string) ClientOption {
	return func(s *transport.Settings) {
		s.MTLSCertFile = certFile
		s.MTLSKeyFile = keyFile
		s.MTLSCACert = caCertFile
	}
}

// WithLogger configures a custom zap logger.
// The logger is passed through to the transport layer.
func WithLogger(logger *zap.Logger) ClientOption {
	return func(s *transport.Settings) { _ = logger } // stored in Client.logger
}

// WithDebug enables verbose request/response logging in resty.
func WithDebug() ClientOption {
	return func(s *transport.Settings) { s.Debug = true }
}
