// Package download provides CDN file download operations for Windows Update files.
package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/client"
	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/shared/models"
	"resty.dev/v3"
)

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
// WithCDNURLs first.
func (s *Service) DownloadFile(ctx context.Context, file models.File, destDir string) (*resty.Response, error) {
	if file.URL == "" {
		return nil, fmt.Errorf("DownloadFile: %q has no CDN URL — call GetFiles with WithCDNURLs first", file.Name)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("DownloadFile: create dest dir: %w", err)
	}
	return s.downloadOne(ctx, file, destDir)
}

// DownloadFiles downloads multiple files concurrently to destDir.
// concurrency controls the number of parallel downloads; 0 defaults to 4.
//
// All files must have a CDN URL (call Files.GetFiles with WithCDNURLs first).
// Returns the first error encountered; remaining in-flight downloads are
// cancelled.
func (s *Service) DownloadFiles(ctx context.Context, files []models.File, destDir string, concurrency int) error {
	if concurrency <= 0 {
		concurrency = 4
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("DownloadFiles: create dest dir: %w", err)
	}

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
				if _, err := s.downloadOne(ctx, j.file, destDir); err != nil {
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
func (s *Service) downloadOne(ctx context.Context, file models.File, destDir string) (*resty.Response, error) {
	if file.URL == "" {
		return nil, fmt.Errorf("download %q: no CDN URL", file.Name)
	}

	logger := s.client.GetLogger()
	dest := filepath.Join(destDir, file.Name)

	// Skip if already fully downloaded (same size).
	if info, err := os.Stat(dest); err == nil && info.Size() == file.SizeBytes && file.SizeBytes > 0 {
		logger.Sugar().Infof("skip %s (already exists, %d bytes)", file.Name, file.SizeBytes)
		return nil, nil
	}

	// SetDoNotParseResponse enables streaming: resty will not buffer the body.
	// Callers must close resp.RawResponse.Body when done.
	resp, err := s.client.GetDownloadClient().R().
		SetContext(ctx).
		SetDoNotParseResponse(true).
		Get(file.URL)
	if err != nil {
		return nil, fmt.Errorf("download %q: HTTP GET: %w", file.Name, err)
	}
	defer resp.RawResponse.Body.Close()

	if resp.StatusCode() != http.StatusOK {
		return resp, fmt.Errorf("download %q: CDN returned HTTP %d", file.Name, resp.StatusCode())
	}

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

	if _, err := io.Copy(tmp, resp.RawResponse.Body); err != nil {
		return resp, fmt.Errorf("download %q: stream: %w", file.Name, err)
	}
	if err := tmp.Close(); err != nil {
		return resp, fmt.Errorf("download %q: close temp: %w", file.Name, err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return resp, fmt.Errorf("download %q: rename: %w", file.Name, err)
	}

	logger.Sugar().Infof("downloaded %s → %s", file.Name, dest)
	return resp, nil
}
