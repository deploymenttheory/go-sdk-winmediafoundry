// Package mock provides a test double for wuproto.WindowsUpdateClient.
package mock

import (
	"context"
	"sync"

	"github.com/deploymenttheory/go-sdk-windowsuup/wuproto"
)

// Client is a configurable mock implementation of wuproto.WindowsUpdateClient.
// Callers set FetchUpdatesFunc and GetFileURLsFunc before use.
//
// All methods record their invocations in Calls for assertion in tests.
type Client struct {
	mu sync.Mutex

	// FetchUpdatesFunc is called by FetchUpdates. If nil, returns nil, nil.
	FetchUpdatesFunc func(ctx context.Context, req wuproto.FetchRequest) ([]wuproto.UpdateResult, error)

	// GetFileURLsFunc is called by GetFileURLs. If nil, returns nil, nil.
	GetFileURLsFunc func(ctx context.Context, req wuproto.FileURLRequest) ([]wuproto.FileURL, error)

	// Calls records every invocation for test assertions.
	Calls []Call
}

// Call records a single method invocation on the mock client.
type Call struct {
	Method     string
	FetchReq   *wuproto.FetchRequest
	FileURLReq *wuproto.FileURLRequest
}

// FetchUpdates implements wuproto.WindowsUpdateClient.
func (c *Client) FetchUpdates(ctx context.Context, req wuproto.FetchRequest) ([]wuproto.UpdateResult, error) {
	c.mu.Lock()
	c.Calls = append(c.Calls, Call{Method: "FetchUpdates", FetchReq: &req})
	fn := c.FetchUpdatesFunc
	c.mu.Unlock()
	if fn == nil {
		return nil, nil
	}
	return fn(ctx, req)
}

// GetFileURLs implements wuproto.WindowsUpdateClient.
func (c *Client) GetFileURLs(ctx context.Context, req wuproto.FileURLRequest) ([]wuproto.FileURL, error) {
	c.mu.Lock()
	c.Calls = append(c.Calls, Call{Method: "GetFileURLs", FileURLReq: &req})
	fn := c.GetFileURLsFunc
	c.mu.Unlock()
	if fn == nil {
		return nil, nil
	}
	return fn(ctx, req)
}

// Reset clears all recorded calls.
func (c *Client) Reset() {
	c.mu.Lock()
	c.Calls = nil
	c.mu.Unlock()
}
