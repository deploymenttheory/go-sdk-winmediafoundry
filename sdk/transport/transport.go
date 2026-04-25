package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"resty.dev/v3"
	"go.uber.org/zap"
)

// Transport wraps a resty.Client with retry, concurrency limiting, and
// mTLS support.
type Transport struct {
	client *resty.Client
	sem    chan struct{} // nil = unlimited
	delay  time.Duration
	mu     sync.Mutex
	logger *zap.Logger
}

// New creates a Transport from the given Settings.
func New(cfg *Settings, logger *zap.Logger) (*Transport, error) {
	if cfg == nil {
		cfg = DefaultSettings()
	}

	tlsConfig, err := buildTLSConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build TLS config: %w", err)
	}

	hc := &http.Client{
		Timeout: cfg.Timeout,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	client := resty.NewWithClient(hc)
	client.SetBaseURL(cfg.BaseURL)
	client.SetHeader("User-Agent", cfg.UserAgent)
	client.SetRetryCount(cfg.RetryCount)
	client.SetRetryWaitTime(cfg.RetryWaitTime)
	client.SetRetryMaxWaitTime(cfg.RetryMaxWaitTime)
	for k, v := range cfg.GlobalHeaders {
		client.SetHeader(k, v)
	}
	if cfg.ProxyURL != "" {
		client.SetProxy(cfg.ProxyURL)
	}

	var sem chan struct{}
	if cfg.MaxConcurrentRequests > 0 {
		sem = make(chan struct{}, cfg.MaxConcurrentRequests)
	}

	return &Transport{
		client: client,
		sem:    sem,
		delay:  cfg.MandatoryRequestDelay,
		logger: logger,
	}, nil
}

// Request creates a new resty request with the given context.
func (t *Transport) Request(ctx context.Context) *resty.Request {
	return t.client.R().SetContext(ctx)
}

// Do executes a pre-built resty.Request, honouring concurrency limits and
// mandatory delays.
func (t *Transport) Do(req *resty.Request) (*resty.Response, error) {
	if t.sem != nil {
		t.sem <- struct{}{}
		defer func() { <-t.sem }()
	}

	resp, err := req.Send()
	if err != nil {
		return nil, err
	}

	if t.delay > 0 {
		time.Sleep(t.delay)
	}

	return resp, nil
}

// buildTLSConfig constructs a *tls.Config from mTLS settings.
func buildTLSConfig(cfg *Settings) (*tls.Config, error) {
	tlsCfg := &tls.Config{
		InsecureSkipVerify: cfg.InsecureSkipVerify, //nolint:gosec
	}

	if cfg.MTLSCertFile != "" && cfg.MTLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.MTLSCertFile, cfg.MTLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load mTLS client cert: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	if cfg.MTLSCACert != "" {
		caPEM, err := os.ReadFile(cfg.MTLSCACert)
		if err != nil {
			return nil, fmt.Errorf("read CA cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("parse CA cert from %s", cfg.MTLSCACert)
		}
		tlsCfg.RootCAs = pool
	}

	return tlsCfg, nil
}

// parseAPIError reads the JSON error envelope from a non-2xx response.
func parseAPIError(resp *resty.Response) error {
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp.Bytes(), &envelope); err != nil {
		return &APIError{
			StatusCode: resp.StatusCode(),
			Code:       "UNKNOWN",
			Message:    string(resp.Bytes()),
		}
	}
	return &APIError{
		StatusCode: resp.StatusCode(),
		Code:       envelope.Error.Code,
		Message:    envelope.Error.Message,
	}
}
