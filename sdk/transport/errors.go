package transport

import (
	"errors"
	"fmt"
	"net/http"
)

// APIError is returned when the server responds with a non-2xx status code.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d (%s): %s", e.StatusCode, e.Code, e.Message)
}

// IsNotFound returns true when err is an APIError with status 404.
func IsNotFound(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.StatusCode == http.StatusNotFound
}

// IsRateLimited returns true when err is an APIError with status 429.
func IsRateLimited(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.StatusCode == http.StatusTooManyRequests
}

// IsBadRequest returns true when err is an APIError with status 400.
func IsBadRequest(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.StatusCode == http.StatusBadRequest
}

// IsServerError returns true when err is an APIError with a 5xx status.
func IsServerError(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.StatusCode >= 500
}
