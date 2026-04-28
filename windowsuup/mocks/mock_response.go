package mocks

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"resty.dev/v3"
)

// NewMockResponse constructs a *resty.Response for use in unit tests.
// The status code, headers, and body are surfaced through resty's normal
// accessors (resp.StatusCode(), resp.Bytes(), resp.RawResponse.Body).
func NewMockResponse(statusCode int, headers http.Header, body []byte) *resty.Response {
	if headers == nil {
		headers = make(http.Header)
	}
	if body == nil {
		body = []byte{}
	}

	status := http.StatusText(statusCode)
	if status == "" {
		status = fmt.Sprintf("%d", statusCode)
	}

	// Use a minimal resty.Request so that resp.Bytes() does not panic when it
	// calls r.Request.DoNotParseResponse.
	req := &resty.Request{
		URL:                "",
		DoNotParseResponse: false,
	}

	return &resty.Response{
		Request: req,
		// Body is read by resp.Bytes() via readIfRequired() when DoNotParseResponse
		// is false. Use a fresh reader each time so the response can be re-read.
		Body: io.NopCloser(bytes.NewReader(body)),
		RawResponse: &http.Response{
			StatusCode: statusCode,
			Status:     status,
			Header:     headers,
			// RawResponse.Body is used by streaming download handlers that call
			// resp.RawResponse.Body directly after SetDoNotParseResponse(true).
			Body: io.NopCloser(bytes.NewReader(body)),
		},
	}
}
