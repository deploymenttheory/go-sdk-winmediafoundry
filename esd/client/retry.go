package client

import (
	"resty.dev/v3"
)

// retryCondition is the resty AddRetryConditions callback.
// It returns true when the request should be retried.
//
// SOAP calls use HTTP POST exclusively, but all Windows Update protocol calls
// are read-only queries with no side effects. POST is therefore treated as
// idempotent for retry purposes, unlike the jamfpro-v2 pattern where POST is
// excluded from retry to prevent duplicate resource creation.
//
// Retry rules:
//   - Transient server errors (408, 500, 502, 503, 504) are retried.
//   - Cookie / config errors on 500 are handled separately at the service layer
//     (invalidate + re-acquire cookie), so they also benefit from retry here.
//   - Definitive client errors (4xx excluding 408) are never retried.
//   - Network-level errors (resp == nil) are retried.
func retryCondition(resp *resty.Response, err error) bool {
	// Network / transport error — retry.
	if err != nil {
		return true
	}

	if resp == nil {
		return false
	}

	code := resp.StatusCode()

	// Never retry definitive client-side failures.
	if isNonRetryableStatusCode(code) {
		return false
	}

	return isTransientStatusCode(code)
}

// isTransientStatusCode returns true for errors that are likely temporary.
func isTransientStatusCode(code int) bool {
	switch code {
	case 408, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

// isNonRetryableStatusCode returns true for definitive client-side errors.
func isNonRetryableStatusCode(code int) bool {
	switch code {
	case 400, 401, 402, 403, 404, 405, 406, 407, 409, 410,
		411, 412, 413, 414, 415, 416, 417, 422, 423, 424,
		426, 428, 429, 431, 451:
		return true
	default:
		return false
	}
}
