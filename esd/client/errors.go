package client

import (
	"encoding/xml"
	"fmt"
	"net/http"

	"go.uber.org/zap"
)

// APIError represents an error response from the Windows Update SOAP API.
type APIError struct {
	Code       string
	Message    string
	StatusCode int
	Status     string
	Endpoint   string
	Method     string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("Windows Update API error (%d %s) [%s] at %s %s: %s",
			e.StatusCode, e.Status, e.Code, e.Method, e.Endpoint, e.Message)
	}
	return fmt.Sprintf("Windows Update API error (%d %s) at %s %s: %s",
		e.StatusCode, e.Status, e.Method, e.Endpoint, e.Message)
}

// soapFault is the minimal XML structure of a SOAP 1.2 fault response.
type soapFault struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		Fault struct {
			Code struct {
				Value    string `xml:"Value"`
				Subcode  struct{ Value string `xml:"Value"` } `xml:"Subcode"`
			} `xml:"Code"`
			Reason struct {
				Text string `xml:"Text"`
			} `xml:"Reason"`
		} `xml:"Fault"`
	} `xml:"Body"`
}

// ParseErrorResponse parses an error response from the SOAP API.
// It attempts to parse a SOAP XML fault first; on failure it uses the raw body.
func ParseErrorResponse(body []byte, statusCode int, status, method, endpoint string, logger *zap.Logger) error {
	apiError := &APIError{
		StatusCode: statusCode,
		Status:     status,
		Endpoint:   endpoint,
		Method:     method,
	}

	// Attempt SOAP XML fault parse.
	var fault soapFault
	if err := xml.Unmarshal(body, &fault); err == nil {
		code := fault.Body.Fault.Code.Value
		sub := fault.Body.Fault.Code.Subcode.Value
		if sub != "" {
			code = sub
		}
		msg := fault.Body.Fault.Reason.Text
		if code != "" || msg != "" {
			apiError.Code = code
			apiError.Message = msg
		}
	}

	if apiError.Message == "" {
		apiError.Message = string(body)
		if apiError.Message == "" {
			apiError.Message = defaultMessageForStatus(statusCode)
		}
	}

	logger.Error("API error response",
		zap.Int("status_code", statusCode),
		zap.String("method", method),
		zap.String("endpoint", endpoint),
		zap.String("message", apiError.Message),
	)
	return apiError
}

func defaultMessageForStatus(statusCode int) string {
	switch statusCode {
	case http.StatusBadRequest:
		return "The request could not be understood by the server due to malformed syntax."
	case http.StatusUnauthorized:
		return "The request lacks valid authentication credentials."
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

// IsUnauthorized reports whether err is a 401 response.
func IsUnauthorized(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == http.StatusUnauthorized
	}
	return false
}

// IsBadRequest reports whether err is a 400 response.
func IsBadRequest(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == http.StatusBadRequest
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
