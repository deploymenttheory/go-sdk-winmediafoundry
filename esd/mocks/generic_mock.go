// Package mocks provides test infrastructure for the Windows Update SDK.
// It supplies a GenericMock that implements client.Client without making real
// network calls, plus a NewMockResponse helper for constructing resty.Response
// fixtures.
package mocks

import (
	"context"
	"fmt"
	"net/http"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/esd/client"
	"go.uber.org/zap"
	"resty.dev/v3"
)

// registeredResponse holds a pre-canned response for a single endpoint.
type registeredResponse struct {
	statusCode int
	body       []byte
	errMsg     string
}

// GenericMock is a test double implementing client.Client.
//
// Register SOAP XML response bodies by HTTP method + endpoint, and configure
// WU session cookie values via SetCookie. Suitable for service-layer unit tests
// that must not make real network calls.
//
// Usage:
//
//	m := mocks.NewGenericMock()
//	m.SetCookie("enc-data", "2099-01-01T00:00:00Z", "dev-token")
//	m.Register("POST", soap.ClientEndpoint, http.StatusOK, fixtureXML)
//
//	svc := builds.New(m)
//	result, _, err := svc.FetchBuilds(context.Background())
type GenericMock struct {
	responses           map[string]registeredResponse
	logger              *zap.Logger
	cookieEncData       string
	cookieExpiry        string
	cookieDevToken      string
	cookieErr           error
	// InvalidateCookieCount records how many times InvalidateWUCookie was called.
	InvalidateCookieCount int
}

// NewGenericMock returns a GenericMock with a no-op logger and empty responses.
func NewGenericMock() *GenericMock {
	return &GenericMock{
		responses: make(map[string]registeredResponse),
		logger:    zap.NewNop(),
	}
}

// Register registers a successful mock response for the given method + endpoint.
func (m *GenericMock) Register(method, endpoint string, statusCode int, body []byte) {
	m.responses[method+":"+endpoint] = registeredResponse{statusCode: statusCode, body: body}
}

// RegisterError registers a mock error response for the given method + endpoint.
func (m *GenericMock) RegisterError(method, endpoint string, statusCode int, errMsg string) {
	m.responses[method+":"+endpoint] = registeredResponse{statusCode: statusCode, errMsg: errMsg}
}

// SetCookie configures the WU session cookie values returned by AcquireWUCookie.
func (m *GenericMock) SetCookie(encryptedData, expiration, deviceToken string) {
	m.cookieEncData = encryptedData
	m.cookieExpiry = expiration
	m.cookieDevToken = deviceToken
}

// SetCookieError causes AcquireWUCookie to return the given error.
func (m *GenericMock) SetCookieError(err error) {
	m.cookieErr = err
}

// NewRequest returns a *client.RequestBuilder backed by this mock's response
// registry. Calls to Get/Post/Put/Delete on the builder look up the registered
// response keyed by "METHOD:path".
func (m *GenericMock) NewRequest(ctx context.Context) *client.RequestBuilder {
	return client.NewMockRequestBuilder(ctx, func(method, path string, _ any) (*resty.Response, error) {
		r, ok := m.responses[method+":"+path]
		if !ok {
			return nil, fmt.Errorf("mock: no response registered for %s %s", method, path)
		}
		headers := http.Header{"Content-Type": {"application/soap+xml; charset=utf-8"}}
		resp := NewMockResponse(r.statusCode, headers, r.body)
		if r.errMsg != "" {
			return resp, fmt.Errorf("%s", r.errMsg)
		}
		return resp, nil
	})
}

// GetLogger returns the mock's no-op logger.
func (m *GenericMock) GetLogger() *zap.Logger { return m.logger }

// AcquireWUCookie returns the pre-configured cookie values.
func (m *GenericMock) AcquireWUCookie(_ context.Context) (string, string, string, error) {
	return m.cookieEncData, m.cookieExpiry, m.cookieDevToken, m.cookieErr
}

// InvalidateWUCookie records the call and is otherwise a no-op.
func (m *GenericMock) InvalidateWUCookie() {
	m.InvalidateCookieCount++
}
