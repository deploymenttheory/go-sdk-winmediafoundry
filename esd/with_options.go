package esd

import (
	"crypto/tls"
	"fmt"
	"maps"
	"net/http"
	"time"

	"github.com/deploymenttheory/winmediafoundry/esd/client"
	"go.uber.org/zap"
)

// ClientOption configures the Windows Update SDK transport at construction time.
// Pass one or more ClientOption values to NewClient.
type ClientOption = client.ClientOption

// WithTimeout sets a custom timeout for HTTP requests.
// Defaults to 2 minutes. CDN downloads can override per-request via
// RequestBuilder.SetTimeout(0).
func WithTimeout(d time.Duration) ClientOption {
	return func(s *client.TransportSettings) error {
		s.Timeout = d
		return nil
	}
}

// WithRetryCount sets the number of retries for failed requests.
func WithRetryCount(count int) ClientOption {
	return func(s *client.TransportSettings) error {
		s.RetryCount = count
		return nil
	}
}

// WithRetryWaitTime sets the wait time between retry attempts.
func WithRetryWaitTime(waitTime time.Duration) ClientOption {
	return func(s *client.TransportSettings) error {
		s.RetryWaitTime = waitTime
		return nil
	}
}

// WithRetryMaxWaitTime sets the maximum wait time between retries.
func WithRetryMaxWaitTime(maxWaitTime time.Duration) ClientOption {
	return func(s *client.TransportSettings) error {
		s.RetryMaxWaitTime = maxWaitTime
		return nil
	}
}

// WithLogger sets a custom zap logger. Returns an error if logger is nil.
func WithLogger(l *zap.Logger) ClientOption {
	return func(s *client.TransportSettings) error {
		if l == nil {
			return fmt.Errorf("logger cannot be nil")
		}
		s.Logger = l
		return nil
	}
}

// WithDebug enables resty's request/response debug logging.
func WithDebug() ClientOption {
	return func(s *client.TransportSettings) error {
		s.Debug = true
		return nil
	}
}

// WithUserAgent sets a custom user-agent string.
func WithUserAgent(userAgent string) ClientOption {
	return func(s *client.TransportSettings) error {
		s.UserAgent = userAgent
		return nil
	}
}

// WithGlobalHeader adds a single header to every outgoing request.
func WithGlobalHeader(key, value string) ClientOption {
	return func(s *client.TransportSettings) error {
		if s.GlobalHeaders == nil {
			s.GlobalHeaders = make(map[string]string)
		}
		s.GlobalHeaders[key] = value
		return nil
	}
}

// WithGlobalHeaders adds multiple headers to every outgoing request.
func WithGlobalHeaders(headers map[string]string) ClientOption {
	return func(s *client.TransportSettings) error {
		if s.GlobalHeaders == nil {
			s.GlobalHeaders = make(map[string]string)
		}
		maps.Copy(s.GlobalHeaders, headers)
		return nil
	}
}

// WithProxy sets an HTTP proxy for all requests.
func WithProxy(proxyURL string) ClientOption {
	return func(s *client.TransportSettings) error {
		s.ProxyURL = proxyURL
		return nil
	}
}

// WithTLSClientConfig sets custom TLS configuration.
// When nil the SDK uses its embedded Microsoft CA bundle plus the system cert
// pool. Ignored when WithInsecureSkipVerify is set.
func WithTLSClientConfig(tlsConfig *tls.Config) ClientOption {
	return func(s *client.TransportSettings) error {
		s.TLSClientConfig = tlsConfig
		return nil
	}
}

// WithTransport sets a custom HTTP transport (http.RoundTripper).
func WithTransport(transport http.RoundTripper) ClientOption {
	return func(s *client.TransportSettings) error {
		s.HTTPTransport = transport
		return nil
	}
}

// WithInsecureSkipVerify disables TLS certificate verification.
// Use only for testing. Takes precedence over WithTLSClientConfig.
func WithInsecureSkipVerify() ClientOption {
	return func(s *client.TransportSettings) error {
		s.InsecureSkipVerify = true
		return nil
	}
}

// WithMaxConcurrentRequests caps the number of parallel in-flight requests.
// Pass 0 to disable limiting (default).
func WithMaxConcurrentRequests(n int) ClientOption {
	return func(s *client.TransportSettings) error {
		s.MaxConcurrentRequests = n
		return nil
	}
}

// WithMandatoryRequestDelay inserts a fixed pause after every successful request.
// Use for bulk operations to avoid rate limiting.
func WithMandatoryRequestDelay(d time.Duration) ClientOption {
	return func(s *client.TransportSettings) error {
		s.MandatoryRequestDelay = d
		return nil
	}
}

// WithTotalRetryDuration sets a maximum wall-clock budget for a request
// including all retry attempts. Requests exceeding this duration are cancelled.
func WithTotalRetryDuration(d time.Duration) ClientOption {
	return func(s *client.TransportSettings) error {
		s.TotalRetryDuration = d
		return nil
	}
}
