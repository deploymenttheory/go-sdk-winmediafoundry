// Package download provides CDN file download operations for Windows Update files.
package download

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/deploymenttheory/winmediafoundry/windowsuup/client"
	"github.com/deploymenttheory/winmediafoundry/windowsuup/shared/models"
	"github.com/deploymenttheory/winmediafoundry/windowsuup/tools/download_counter"
	"go.uber.org/zap"
	"resty.dev/v3"
)

// DownloadProgressCallback is called periodically during a download with the
// current byte count, total expected size, and elapsed time. It matches the
// signature of download_counter.Bar.Callback so callers can pass that directly
// via WithProgressCallback, or use the higher-level WithProgress option.
type DownloadProgressCallback func(fileName string, written, total int64, elapsed time.Duration)

// downloadConfig holds resolved download options.
type downloadConfig struct {
	progressCallback DownloadProgressCallback
}

// DownloadOption configures a download operation.
type DownloadOption func(*downloadConfig)

// WithProgress writes a terminal progress bar to w during each download.
// Pass nil to write to os.Stderr.
func WithProgress(w io.Writer) DownloadOption {
	return func(cfg *downloadConfig) {
		bar := download_counter.New(w)
		cfg.progressCallback = bar.Callback
	}
}

// WithProgressCallback sets a custom progress callback. This is the lower-level
// option; use WithProgress for the built-in terminal bar.
func WithProgressCallback(fn DownloadProgressCallback) DownloadOption {
	return func(cfg *downloadConfig) {
		cfg.progressCallback = fn
	}
}

// Service provides CDN file download operations.
type Service struct {
	client client.Client
}

// New returns a Service backed by the given client transport.
func New(c client.Client) *Service {
	return &Service{client: c}
}

// DownloadFile streams a single file from its CDN URL to destDir.
// The file is written atomically: first to a temp file, then renamed.
//
// Returns an error if file.URL is empty — call Files.GetFiles with
// WithCDNURLs first. Returns an error if the CDN URL has already expired
// (CDN URLs are valid for approximately 12 minutes from resolution).
func (s *Service) DownloadFile(ctx context.Context, file models.File, destDir string, opts ...DownloadOption) (*resty.Response, error) {
	if file.URL == "" {
		return nil, fmt.Errorf("DownloadFile: %q has no CDN URL — call GetFiles with WithCDNURLs first", file.Name)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("DownloadFile: create dest dir: %w", err)
	}
	cfg := applyOptions(opts)
	return s.downloadOne(ctx, file, destDir, cfg)
}

// DownloadFiles downloads multiple files concurrently to destDir.
// concurrency controls the number of parallel downloads; 0 defaults to 4.
//
// All files must have a CDN URL (call Files.GetFiles with WithCDNURLs first).
// Files whose CDN URL has expired are skipped with an error logged; the first
// hard error cancels remaining downloads.
func (s *Service) DownloadFiles(ctx context.Context, files []models.File, destDir string, concurrency int, opts ...DownloadOption) error {
	if concurrency <= 0 {
		concurrency = 4
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("DownloadFiles: create dest dir: %w", err)
	}

	cfg := applyOptions(opts)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type job struct{ file models.File }
	jobs := make(chan job, len(files))
	for _, f := range files {
		jobs <- job{f}
	}
	close(jobs)

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)

	for range concurrency {
		wg.Go(func() {
			for j := range jobs {
				if ctx.Err() != nil {
					return
				}
				if _, err := s.downloadOne(ctx, j.file, destDir, cfg); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
						cancel()
					}
					mu.Unlock()
					return
				}
			}
		})
	}

	wg.Wait()
	return firstErr
}

// downloadOne streams a single file from its CDN URL to destDir atomically.
func (s *Service) downloadOne(ctx context.Context, file models.File, destDir string, cfg *downloadConfig) (*resty.Response, error) {
	if file.URL == "" {
		return nil, fmt.Errorf("download %q: no CDN URL", file.Name)
	}

	// Guard against expired CDN URLs (~12-minute window from EUI2 resolution).
	if !file.ExpiresAt.IsZero() && time.Now().After(file.ExpiresAt) {
		return nil, fmt.Errorf("download %q: CDN URL expired at %s — re-resolve with GetFiles(WithCDNURLs)",
			file.Name, file.ExpiresAt.UTC().Format(time.RFC3339))
	}

	logger := s.client.GetLogger()
	dest := filepath.Join(destDir, file.Name)

	// Skip if already fully downloaded (same size).
	if info, err := os.Stat(dest); err == nil && info.Size() == file.SizeBytes && file.SizeBytes > 0 {
		logger.Info("skipping file (already exists)",
			zap.String("file", file.Name),
			zap.Int64("size_bytes", file.SizeBytes),
		)
		return nil, nil
	}

	// SetDoNotParseResponse enables streaming: resty will not buffer the body.
	// SetTimeout(0) removes the per-request timeout for large file transfers.
	resp, err := s.client.NewRequest(ctx).
		SetDoNotParseResponse(true).
		SetTimeout(0).
		Get(file.URL)
	if err != nil {
		return nil, fmt.Errorf("download %q: %w", file.Name, err)
	}
	defer resp.RawResponse.Body.Close()

	// Write atomically: temp file → rename.
	tmp, err := os.CreateTemp(destDir, ".dl-*.tmp")
	if err != nil {
		return resp, fmt.Errorf("download %q: create temp file: %w", file.Name, err)
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName) // no-op if already renamed
	}()

	// Wrap body with progress reporting when a callback is configured.
	var body io.Reader = resp.RawResponse.Body
	if cfg.progressCallback != nil {
		body = &progressReader{
			src:      resp.RawResponse.Body,
			fileName: file.Name,
			total:    file.SizeBytes,
			start:    time.Now(),
			cb:       cfg.progressCallback,
		}
	}

	if _, err := io.Copy(tmp, body); err != nil {
		return resp, fmt.Errorf("download %q: stream: %w", file.Name, err)
	}
	if err := tmp.Close(); err != nil {
		return resp, fmt.Errorf("download %q: close temp: %w", file.Name, err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return resp, fmt.Errorf("download %q: rename: %w", file.Name, err)
	}

	logger.Info("file downloaded",
		zap.String("file", file.Name),
		zap.String("dest", dest),
	)
	return resp, nil
}

// progressReader wraps an io.Reader and calls cb on every read with cumulative
// byte counts and elapsed time. Each downloadOne has its own progressReader so
// no synchronisation is needed on the written counter.
type progressReader struct {
	src      io.Reader
	fileName string
	total    int64
	written  int64
	start    time.Time
	cb       DownloadProgressCallback
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if n > 0 {
		r.written += int64(n)
		r.cb(r.fileName, r.written, r.total, time.Since(r.start))
	}
	return n, err
}

// applyOptions resolves DownloadOption values into a downloadConfig.
func applyOptions(opts []DownloadOption) *downloadConfig {
	cfg := &downloadConfig{}
	for _, o := range opts {
		o(cfg)
	}
	return cfg
}
