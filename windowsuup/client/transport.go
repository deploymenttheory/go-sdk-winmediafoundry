package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/deploymenttheory/go-sdk-windowsuup/internal/wuproto"
	"github.com/deploymenttheory/go-sdk-windowsuup/internal/wuproto/soap"
	"go.uber.org/zap"
	"resty.dev/v3"
)

// Transport is the concrete implementation of Client.
// It wraps the internal SOAP client (for Windows Update protocol calls) and a
// separate resty client (for CDN file downloads).
//
// Transport is safe for concurrent use.
type Transport struct {
	soap   *soap.SOAPClient
	dl     *resty.Client
	logger *zap.Logger
}

// NewTransport constructs a Transport from the given TransportSettings.
// It eagerly acquires a Windows Update session cookie so connectivity failures
// are surfaced at construction time rather than on the first SDK call.
func NewTransport(settings *TransportSettings) (*Transport, error) {
	logger := settings.Logger
	if logger == nil {
		var err error
		logger, err = zap.NewProduction()
		if err != nil {
			return nil, fmt.Errorf("create default logger: %w", err)
		}
	}

	// ── SOAP resty client (with timeout) ────────────────────────────────────
	soapOpts := []soap.Option{
		soap.WithTimeout(settings.Timeout),
	}
	if settings.TLSConfig != nil {
		soapOpts = append(soapOpts, soap.WithTLSConfig(settings.TLSConfig))
	}
	if settings.HTTPClient != nil {
		soapOpts = append(soapOpts, soap.WithHTTPClient(settings.HTTPClient))
	}

	soapClient, err := soap.New(logger, soapOpts...)
	if err != nil {
		return nil, fmt.Errorf("create SOAP client: %w", err)
	}

	// ── CDN download resty client (no read timeout — large files) ───────────
	// SetDoNotParseResponse is applied per-request in the download service to
	// enable streaming. The client-level setting is left false so SOAP error
	// responses (which reuse the same transport) are still auto-read.
	var dlClient *resty.Client
	if settings.HTTPClient != nil {
		dlClient = resty.NewWithClient(settings.HTTPClient)
	} else {
		hc := &http.Client{
			Transport: &http.Transport{},
		}
		if settings.TLSConfig != nil {
			hc.Transport = &http.Transport{
				TLSClientConfig: settings.TLSConfig,
			}
		}
		dlClient = resty.NewWithClient(hc)
	}

	return &Transport{
		soap:   soapClient,
		dl:     dlClient,
		logger: logger,
	}, nil
}

// FetchUpdates implements Client.
func (t *Transport) FetchUpdates(ctx context.Context, req wuproto.FetchRequest) ([]wuproto.UpdateResult, *resty.Response, error) {
	return t.soap.FetchUpdates(ctx, req)
}

// GetFileURLs implements Client.
func (t *Transport) GetFileURLs(ctx context.Context, req wuproto.FileURLRequest) ([]wuproto.FileURL, *resty.Response, error) {
	return t.soap.GetFileURLs(ctx, req)
}

// GetDownloadClient implements Client.
func (t *Transport) GetDownloadClient() *resty.Client {
	return t.dl
}

// GetLogger implements Client.
func (t *Transport) GetLogger() *zap.Logger {
	return t.logger
}
