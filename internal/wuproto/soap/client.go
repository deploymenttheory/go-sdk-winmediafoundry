// Package soap provides a concrete implementation of
// wuproto.WindowsUpdateClient using the Windows Update SOAP protocol.
//
// It handles device token generation, encrypted session cookie acquisition and
// caching, and both SyncUpdates (FetchUpdates) and GetExtendedUpdateInfo2
// (GetFileURLs) SOAP calls.
package soap

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/deploymenttheory/go-sdk-windowsuup/internal/wuproto"
	"go.uber.org/zap"
	"resty.dev/v3"
)

// SOAPClient is a wuproto.WindowsUpdateClient that speaks the Windows Update
// SOAP protocol directly.
//
// Create one with New. SOAPClient is safe for concurrent use.
type SOAPClient struct {
	cookies *cookieManager
	logger  *zap.Logger
}

// Option configures a SOAPClient.
type Option func(*config)

type config struct {
	timeout    time.Duration
	tlsConfig  *tls.Config
	httpClient *http.Client
}

func defaultConfig() *config {
	return &config{
		timeout: 60 * time.Second,
	}
}

// WithTimeout sets the per-request HTTP timeout (default 60 s).
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// WithTLSConfig replaces the TLS configuration used by the HTTP client.
func WithTLSConfig(cfg *tls.Config) Option {
	return func(c *config) { c.tlsConfig = cfg }
}

// WithHTTPClient replaces the HTTP client entirely (overrides timeout and TLS).
func WithHTTPClient(hc *http.Client) Option {
	return func(c *config) { c.httpClient = hc }
}

// New creates a SOAPClient, generating a device token and acquiring the first
// WU session cookie eagerly (to surface auth failures early).
func New(logger *zap.Logger, opts ...Option) (*SOAPClient, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(cfg)
	}

	var rc *resty.Client
	if cfg.httpClient != nil {
		// Caller-supplied http.Client: wrap it in resty.
		rc = resty.NewWithClient(cfg.httpClient)
	} else {
		// Build a TLS-aware http.Client and wrap it in resty.
		tlsCfg := cfg.tlsConfig
		if tlsCfg == nil {
			pool, err := wuCertPool()
			if err != nil {
				return nil, fmt.Errorf("build TLS cert pool: %w", err)
			}
			tlsCfg = &tls.Config{RootCAs: pool}
		}
		hc := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig:   tlsCfg,
				DisableKeepAlives: false,
			},
		}
		rc = resty.NewWithClient(hc)
	}

	// Apply SOAP defaults that every request inherits.
	rc.SetTimeout(cfg.timeout).
		SetHeader("Content-Type", "application/soap+xml; charset=utf-8").
		SetHeader("User-Agent", userAgent)

	cm, err := newCookieManager(rc, logger)
	if err != nil {
		return nil, err
	}

	return &SOAPClient{
		cookies: cm,
		logger:  logger,
	}, nil
}

// FetchUpdates implements wuproto.WindowsUpdateClient.
func (c *SOAPClient) FetchUpdates(ctx context.Context, req wuproto.FetchRequest) ([]wuproto.UpdateResult, *resty.Response, error) {
	return c.fetchUpdates(ctx, req)
}

// GetFileURLs implements wuproto.WindowsUpdateClient.
func (c *SOAPClient) GetFileURLs(ctx context.Context, req wuproto.FileURLRequest) ([]wuproto.FileURL, *resty.Response, error) {
	return c.getFileURLs(ctx, req)
}
