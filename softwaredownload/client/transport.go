package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/constants"
	"go.uber.org/zap"
	"resty.dev/v3"
)

// Transport is the concrete implementation of Client. It wraps a resty.Client
// (resty.dev/v3) with SDK behaviour: transient-aware retry with exponential
// backoff, adaptive response-time throttling, optional concurrency limiting,
// and structured logging.
//
// Transport is safe for concurrent use.
type Transport struct {
	client          *resty.Client
	logger          *zap.Logger
	globalHeaders   map[string]string
	userAgent       string
	sem             *semaphore
	requestDelay    time.Duration
	totalRetryDur   time.Duration
	responseTracker *responseTimeTracker
}

// GetLogger returns the configured logger.
func (t *Transport) GetLogger() *zap.Logger {
	return t.logger
}

// NewRequest returns a RequestBuilder for this transport. The service layer
// uses it to construct the full request — headers, query params, body —
// before calling Get/Post to execute it. Retry, concurrency limiting, and
// throttling are applied by the transport.
func (t *Transport) NewRequest(ctx context.Context) *RequestBuilder {
	return &RequestBuilder{
		req:      t.client.R().SetContext(ctx).SetResponseBodyUnlimitedReads(true),
		executor: t,
	}
}

// NewTransport creates and fully configures a software-download SDK transport.
//
// Behaviour applied at construction time:
//   - Transient-aware retry with exponential backoff
//   - Adaptive inter-request delay derived from response-time EMA tracking
//   - Optional concurrency limiting
//   - OpenTelemetry instrumentation (no-op when no global provider configured)
func NewTransport(settings *TransportSettings) (*Transport, error) {
	// Logger: caller-supplied or production default.
	logger := settings.Logger
	if logger == nil {
		var err error
		logger, err = zap.NewProduction()
		if err != nil {
			return nil, fmt.Errorf("create default logger: %w", err)
		}
	}

	// User agent: option overrides SDK default.
	userAgent := settings.UserAgent
	if userAgent == "" {
		userAgent = fmt.Sprintf("%s/%s", UserAgentBase, constants.Version)
	}

	// Timeouts/retries: option value if non-zero, else SDK default.
	timeout := settings.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	retryCount := settings.RetryCount
	if retryCount == 0 {
		retryCount = MaxRetries
	}
	retryWait := settings.RetryWaitTime
	if retryWait == 0 {
		retryWait = RetryWaitTime
	}
	retryMaxWait := settings.RetryMaxWaitTime
	if retryMaxWait == 0 {
		retryMaxWait = RetryMaxWaitTime
	}

	// Build the resty (v3) client.
	rc := resty.New()
	rc.SetTimeout(timeout)
	rc.SetRetryCount(retryCount)
	rc.SetRetryWaitTime(retryWait)
	rc.SetRetryMaxWaitTime(retryMaxWait)
	rc.SetHeader("User-Agent", userAgent)
	rc.AddRetryConditions(retryCondition)

	if settings.Debug {
		rc.SetDebug(true)
	}

	// TLS: InsecureSkipVerify takes precedence over a custom TLSClientConfig.
	// When neither is set the resty default (system cert pool) is used, which
	// trusts Microsoft's public software-download endpoints.
	if settings.InsecureSkipVerify {
		rc.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}) //nolint:gosec
	} else if settings.TLSClientConfig != nil {
		rc.SetTLSClientConfig(settings.TLSClientConfig)
	}

	if settings.ProxyURL != "" {
		rc.SetProxy(settings.ProxyURL)
	}
	if settings.HTTPTransport != nil {
		rc.SetTransport(settings.HTTPTransport)
	}
	if settings.GlobalHeaders == nil {
		settings.GlobalHeaders = make(map[string]string)
	}
	for k, v := range settings.GlobalHeaders {
		rc.SetHeader(k, v)
	}

	// Build optional concurrency semaphore.
	var sem *semaphore
	if settings.MaxConcurrentRequests > 0 {
		sem = newSemaphore(settings.MaxConcurrentRequests)
	}

	transport := &Transport{
		client:          rc,
		logger:          logger,
		globalHeaders:   settings.GlobalHeaders,
		userAgent:       userAgent,
		responseTracker: newResponseTimeTracker(),
		sem:             sem,
		requestDelay:    settings.MandatoryRequestDelay,
		totalRetryDur:   settings.TotalRetryDuration,
	}

	// Apply OpenTelemetry instrumentation (no-op when no global provider is set).
	transport.applyOpenTelemetry()

	logger.Info("software-download SDK transport created",
		zap.String("user_agent", userAgent),
	)
	return transport, nil
}

// execute implements requestExecutor for Transport.
func (t *Transport) execute(req *resty.Request, method, path string, _ any) (*resty.Response, error) {
	return t.executeRequest(req, method, path)
}

// executeGetBytes implements requestExecutor for Transport.
func (t *Transport) executeGetBytes(req *resty.Request, path string) (*resty.Response, []byte, error) {
	resp, err := t.executeRequest(req, "GET", path)
	if err != nil {
		return resp, nil, err
	}
	return resp, resp.Bytes(), nil
}

// executeRequest is the central request executor. It applies the concurrency
// semaphore, total-retry deadline, mandatory per-request delay, and adaptive
// response-time throttling.
func (t *Transport) executeRequest(req *resty.Request, method, path string) (*resty.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Wrap in a deadline for the total allowed retry window if configured.
	if t.totalRetryDur > 0 {
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, t.totalRetryDur)
			defer cancel()
			req.SetContext(ctx)
		}
	}

	// Acquire concurrency slot — blocks until available or context cancelled.
	if t.sem != nil {
		if err := t.sem.acquire(ctx); err != nil {
			return nil, fmt.Errorf("concurrency limit: %w", err)
		}
		defer t.sem.release()
	}

	t.logger.Debug("Executing request",
		zap.String("method", method),
		zap.String("path", path),
	)

	resp, execErr := req.Execute(method, path)
	if execErr != nil {
		t.logger.Error("Request failed",
			zap.String("method", method),
			zap.String("path", path),
			zap.Error(execErr),
		)
		return resp, fmt.Errorf("request failed: %w", execErr)
	}

	if resp.IsStatusFailure() {
		return resp, ParseErrorResponse(
			resp.Bytes(),
			resp.StatusCode(),
			resp.Status(),
			method,
			path,
			t.logger,
		)
	}

	duration := resp.Duration()

	t.logger.Info("Request completed",
		zap.String("method", method),
		zap.String("path", path),
		zap.Int("status_code", resp.StatusCode()),
		zap.Duration("duration", duration),
	)

	// Mandatory fixed delay (user-configured for bulk operations).
	if t.requestDelay > 0 {
		time.Sleep(t.requestDelay)
	}

	// Adaptive delay: pause when the server is responding more slowly than its
	// own EMA baseline.
	if adaptive := t.responseTracker.record(duration); adaptive > 0 {
		t.logger.Debug("Adaptive delay applied due to elevated response time",
			zap.Duration("response_time", duration),
			zap.Duration("adaptive_delay", adaptive),
		)
		time.Sleep(adaptive)
	}

	return resp, nil
}
