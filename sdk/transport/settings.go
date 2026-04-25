// Package transport provides the HTTP transport layer for the SDK client,
// including retry logic, concurrency limiting, mTLS, and rate-limit handling.
package transport

import "time"

// Settings holds all configuration for the SDK transport layer.
type Settings struct {
	// BaseURL is the HTTP API server base URL.
	// Default: "https://localhost:8443"
	BaseURL string

	// Timeout is the per-request timeout.
	// Default: 60 s
	Timeout time.Duration

	// RetryCount is the number of retry attempts for transient failures.
	// Default: 3
	RetryCount int

	// RetryWaitTime is the minimum wait between retries.
	// Default: 2 s
	RetryWaitTime time.Duration

	// RetryMaxWaitTime is the maximum wait between retries (exponential backoff ceiling).
	// Default: 30 s
	RetryMaxWaitTime time.Duration

	// TotalRetryDuration, when > 0, sets a wall-clock budget for a request
	// including all retries. Default: disabled (0).
	TotalRetryDuration time.Duration

	// MaxConcurrentRequests, when > 0, caps the number of in-flight requests.
	// Default: unlimited (0).
	MaxConcurrentRequests int

	// MandatoryRequestDelay, when > 0, introduces a fixed pause after every
	// successful request. Default: none (0).
	MandatoryRequestDelay time.Duration

	// UserAgent overrides the default User-Agent header.
	UserAgent string

	// GlobalHeaders are added to every request.
	GlobalHeaders map[string]string

	// ProxyURL routes all requests through an HTTP proxy.
	ProxyURL string

	// InsecureSkipVerify disables TLS certificate verification. Testing only.
	InsecureSkipVerify bool

	// mTLS fields — configure client certificate authentication.
	MTLSCertFile string // path to PEM-encoded client certificate
	MTLSKeyFile  string // path to PEM-encoded private key
	MTLSCACert   string // path to PEM-encoded CA certificate for server verification
}

// DefaultSettings returns a Settings with sensible defaults.
func DefaultSettings() *Settings {
	return &Settings{
		BaseURL:          "https://localhost:8443",
		Timeout:          60 * time.Second,
		RetryCount:       3,
		RetryWaitTime:    2 * time.Second,
		RetryMaxWaitTime: 30 * time.Second,
		UserAgent:        "go-sdk-uupdump/1.0",
	}
}
