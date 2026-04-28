package client

import (
	"context"
	"time"

	"resty.dev/v3"
)

// requestExecutor is the execution backend for a RequestBuilder.
// Transport implements it directly; tests supply a mock via NewMockRequestBuilder.
type requestExecutor interface {
	execute(req *resty.Request, method, path string, result any) (*resty.Response, error)
	executeGetBytes(req *resty.Request, path string) (*resty.Response, []byte, error)
}

// RequestBuilder constructs a single API request. The service layer owns the
// full request shape — headers, body, query params — before handing the
// completed request to the executor (transport) which handles retry,
// concurrency limiting, and throttling.
//
// Usage:
//
//	resp, err := s.client.NewRequest(ctx).
//	    SetHeader("Content-Type", constants.ApplicationSOAPXML).
//	    SetHeader("SOAPAction", soap.SyncUpdatesAction).
//	    SetBody(envelope).
//	    Post(soap.ClientEndpoint)
type RequestBuilder struct {
	req      *resty.Request
	executor requestExecutor
	result   any
}

// SetHeader sets a request-level header. Empty values are ignored.
func (b *RequestBuilder) SetHeader(key, value string) *RequestBuilder {
	if value != "" {
		b.req.SetHeader(key, value)
	}
	return b
}

// SetQueryParam adds a URL query parameter. Empty values are ignored.
func (b *RequestBuilder) SetQueryParam(key, value string) *RequestBuilder {
	if value != "" {
		b.req.SetQueryParam(key, value)
	}
	return b
}

// SetBody sets the request body. Nil is ignored.
func (b *RequestBuilder) SetBody(body any) *RequestBuilder {
	if body != nil {
		b.req.SetBody(body)
	}
	return b
}

// SetResult sets the target for response unmarshaling on success.
func (b *RequestBuilder) SetResult(result any) *RequestBuilder {
	b.result = result
	b.req.SetResult(result)
	return b
}

// SetDoNotParseResponse controls whether resty buffers the response body.
// Set to true for streaming large downloads so the caller can read
// resp.RawResponse.Body directly.
func (b *RequestBuilder) SetDoNotParseResponse(val bool) *RequestBuilder {
	b.req.SetDoNotParseResponse(val)
	return b
}

// SetTimeout overrides the client-level timeout for this specific request.
// Pass 0 to disable the timeout (useful for large file downloads).
func (b *RequestBuilder) SetTimeout(d time.Duration) *RequestBuilder {
	b.req.SetTimeout(d)
	return b
}

// Get executes the request as GET against path.
func (b *RequestBuilder) Get(path string) (*resty.Response, error) {
	return b.executor.execute(b.req, "GET", path, b.result)
}

// Post executes the request as POST against path.
func (b *RequestBuilder) Post(path string) (*resty.Response, error) {
	return b.executor.execute(b.req, "POST", path, b.result)
}

// Put executes the request as PUT against path.
func (b *RequestBuilder) Put(path string) (*resty.Response, error) {
	return b.executor.execute(b.req, "PUT", path, b.result)
}

// Delete executes the request as DELETE against path.
func (b *RequestBuilder) Delete(path string) (*resty.Response, error) {
	return b.executor.execute(b.req, "DELETE", path, b.result)
}

// GetBytes executes a GET request and returns raw response bytes without
// unmarshaling. Use for binary responses or raw SOAP XML.
func (b *RequestBuilder) GetBytes(path string) (*resty.Response, []byte, error) {
	return b.executor.executeGetBytes(b.req, path)
}

// ── Mock support ─────────────────────────────────────────────────────────────

// mockRequestExecutor backs a RequestBuilder in tests, routing execution
// through a caller-supplied dispatch function instead of a real Transport.
type mockRequestExecutor struct {
	fn              func(method, path string, result any) (*resty.Response, error)
	queryParamStore *map[string]string
}

func (m *mockRequestExecutor) execute(req *resty.Request, method, path string, result any) (*resty.Response, error) {
	m.captureQueryParams(req)
	return m.fn(method, path, result)
}

func (m *mockRequestExecutor) executeGetBytes(req *resty.Request, path string) (*resty.Response, []byte, error) {
	m.captureQueryParams(req)
	resp, err := m.fn("GET", path, nil)
	if err != nil {
		return resp, nil, err
	}
	return resp, resp.Bytes(), nil
}

func (m *mockRequestExecutor) captureQueryParams(req *resty.Request) {
	if m.queryParamStore != nil && req != nil {
		params := make(map[string]string)
		for k, v := range req.QueryParams {
			if len(v) > 0 {
				params[k] = v[0]
			}
		}
		if len(params) > 0 {
			*m.queryParamStore = params
		}
	}
}

// NewMockRequestBuilder returns a RequestBuilder suitable for unit tests.
// The fn callback receives the HTTP method, path, and result pointer and
// returns a pre-programmed response.
func NewMockRequestBuilder(ctx context.Context, fn func(method, path string, result any) (*resty.Response, error)) *RequestBuilder {
	return &RequestBuilder{
		req:      resty.New().R().SetContext(ctx),
		executor: &mockRequestExecutor{fn: fn},
	}
}

// NewMockRequestBuilderWithQueryCapture returns a RequestBuilder for unit tests
// that also captures query parameters into the provided map pointer.
func NewMockRequestBuilderWithQueryCapture(ctx context.Context, fn func(method, path string, result any) (*resty.Response, error), queryStore *map[string]string) *RequestBuilder {
	return &RequestBuilder{
		req:      resty.New().R().SetContext(ctx),
		executor: &mockRequestExecutor{fn: fn, queryParamStore: queryStore},
	}
}
