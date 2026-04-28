package client

import "time"

const (
	// UserAgentBase is the base string for the SDK user-agent header.
	UserAgentBase = "go-sdk-windowsuup"
)

// HTTP client defaults.
const (
	DefaultTimeout   = 2 * time.Minute
	MaxRetries       = 3
	RetryWaitTime    = 2 * time.Second
	RetryMaxWaitTime = 30 * time.Second

	// DefaultMaxConcurrentRequests caps parallel in-flight requests.
	// 0 means no limit. Set via WithMaxConcurrentRequests.
	DefaultMaxConcurrentRequests = 0

	// adaptiveDelayMax is the ceiling applied to the adaptive inter-request
	// delay computed from response-time EMA tracking.
	adaptiveDelayMax = 5 * time.Second
)
