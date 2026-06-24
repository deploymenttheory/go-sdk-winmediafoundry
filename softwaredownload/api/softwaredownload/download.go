package softwaredownload

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/shared/models"
	"go.uber.org/zap"
	"resty.dev/v3"
)

// Download streams a resolved ISO link to destDir (created if needed) and
// returns the *resty.Response of the transfer. The file is written atomically
// (temp file then rename). Progress can be reported via WithProgress or
// WithProgressCallback.
//
// Download is the standalone form; GetByID/GetByName perform the same transfer
// inline when given WithDownloadDir.
func (s *Service) Download(ctx context.Context, link models.DownloadLink, destDir string, opts ...Option) (*resty.Response, error) {
	cfg := applyOptions(opts)
	_, resp, err := s.downloadOne(ctx, link, destDir, cfg)
	return resp, err
}

// downloadOne streams link to destDir atomically and returns the final path and
// the transfer response.
func (s *Service) downloadOne(ctx context.Context, link models.DownloadLink, destDir string, cfg *config) (string, *resty.Response, error) {
	if link.URL == "" {
		return "", nil, fmt.Errorf("softwaredownload: download link has no URL — resolve it with GetByID/GetByName first")
	}
	if !link.ExpiresAt.IsZero() && time.Now().After(link.ExpiresAt) {
		return "", nil, fmt.Errorf("softwaredownload: download link for %q expired at %s — re-resolve it",
			link.FileName, link.ExpiresAt.UTC().Format(time.RFC3339))
	}

	fileName := link.FileName
	if fileName == "" {
		fileName = fileNameFromURL(link.URL)
	}
	if fileName == "" {
		return "", nil, fmt.Errorf("softwaredownload: cannot determine a file name for %q", link.URL)
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", nil, fmt.Errorf("softwaredownload: create dest dir: %w", err)
	}
	dest := filepath.Join(destDir, fileName)

	logger := s.client.GetLogger()

	// Skip if already present at the expected size (when known).
	if info, err := os.Stat(dest); err == nil && link.SizeBytes > 0 && info.Size() == link.SizeBytes {
		logger.Info("skipping ISO (already exists)",
			zap.String("file", fileName),
			zap.Int64("size_bytes", link.SizeBytes),
		)
		return dest, nil, nil
	}

	// Stream the body: SetDoNotParseResponse avoids buffering, SetTimeout(0)
	// removes the per-request timeout for the multi-GB transfer.
	resp, err := s.client.NewRequest(ctx).
		SetHeader("User-Agent", browserUserAgent).
		SetDoNotParseResponse(true).
		SetTimeout(0).
		Get(link.URL)
	if err != nil {
		return "", resp, fmt.Errorf("softwaredownload: download %q: %w", fileName, err)
	}
	defer resp.RawResponse.Body.Close()

	tmp, err := os.CreateTemp(destDir, ".dl-*.tmp")
	if err != nil {
		return "", resp, fmt.Errorf("softwaredownload: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName) // no-op once renamed
	}()

	total := link.SizeBytes
	if total == 0 {
		total = resp.RawResponse.ContentLength
	}

	var body io.Reader = resp.RawResponse.Body
	if cfg.progressCallback != nil {
		body = &progressReader{
			src:      resp.RawResponse.Body,
			fileName: fileName,
			total:    total,
			start:    time.Now(),
			cb:       cfg.progressCallback,
		}
	}

	if _, err := io.Copy(tmp, body); err != nil {
		return "", resp, fmt.Errorf("softwaredownload: stream %q: %w", fileName, err)
	}
	if err := tmp.Close(); err != nil {
		return "", resp, fmt.Errorf("softwaredownload: close temp %q: %w", fileName, err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return "", resp, fmt.Errorf("softwaredownload: finalize %q: %w", fileName, err)
	}

	logger.Info("ISO downloaded",
		zap.String("file", fileName),
		zap.String("dest", dest),
	)
	return dest, resp, nil
}

// newSessionID returns a random RFC-4122 v4 GUID string, as Microsoft's download
// flow expects for the session identifier. It avoids any external uuid
// dependency by formatting crypto/rand bytes directly.
func newSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// progressReader wraps an io.Reader and calls cb on every read with cumulative
// byte counts and elapsed time.
type progressReader struct {
	src      io.Reader
	fileName string
	total    int64
	written  int64
	start    time.Time
	cb       ProgressCallback
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if n > 0 {
		r.written += int64(n)
		r.cb(r.fileName, r.written, r.total, time.Since(r.start))
	}
	return n, err
}
