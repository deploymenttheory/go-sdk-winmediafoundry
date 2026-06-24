package client

import (
	"fmt"
	"net/http"

	"go.uber.org/zap"
)

// APIError represents a non-2xx HTTP response from a Microsoft software-download
// endpoint (the download pages or the download-connector API).
type APIError struct {
	Message    string
	StatusCode int
	Status     string
	Endpoint   string
	Method     string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("software-download API error (%d %s) at %s %s: %s",
		e.StatusCode, e.Status, e.Method, e.Endpoint, e.Message)
}

// ParseErrorResponse builds an APIError from a failed HTTP response. The body
// is used verbatim as the message (these endpoints return HTML or JSON, not a
// structured fault), falling back to a generic per-status description.
func ParseErrorResponse(body []byte, statusCode int, status, method, endpoint string, logger *zap.Logger) error {
	apiError := &APIError{
		StatusCode: statusCode,
		Status:     status,
		Endpoint:   endpoint,
		Method:     method,
		Message:    string(body),
	}
	if apiError.Message == "" {
		apiError.Message = defaultMessageForStatus(statusCode)
	}

	logger.Error("API error response",
		zap.Int("status_code", statusCode),
		zap.String("method", method),
		zap.String("endpoint", endpoint),
	)
	return apiError
}

func defaultMessageForStatus(statusCode int) string {
	switch statusCode {
	case http.StatusBadRequest:
		return "The request could not be understood by the server due to malformed syntax."
	case http.StatusForbidden:
		return "The server understood the request but refuses to authorize it."
	case http.StatusNotFound:
		return "The server has not found anything matching the Request-URI."
	case http.StatusInternalServerError:
		return "The server encountered an unexpected condition."
	case http.StatusServiceUnavailable:
		return "The server is currently unable to handle the request."
	default:
		return "Unknown error"
	}
}

// IsNotFound reports whether err is a 404 response.
func IsNotFound(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == http.StatusNotFound
	}
	return false
}

// IsForbidden reports whether err is a 403 response. Microsoft returns 403 (or
// an HTML ban page) when the caller's IP is rate-limited or geo-blocked.
func IsForbidden(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == http.StatusForbidden
	}
	return false
}

// IsServerError reports whether err is a 5xx response.
func IsServerError(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode >= http.StatusInternalServerError && apiErr.StatusCode < 600
	}
	return false
}
