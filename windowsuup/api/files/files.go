// Package files provides Windows Update file resolution operations.
package files

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/deploymenttheory/go-sdk-windowsuup/internal/wuproto"
	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/client"
	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/shared/models"
	"resty.dev/v3"
)

// Service provides Windows Update file resolution operations.
type Service struct {
	client client.Client
}

// New returns a Service backed by the given client transport.
func New(c client.Client) *Service {
	return &Service{client: c}
}

// GetFiles retrieves the file list for the given build.
// Pass WithCDNURLs to resolve live CDN download URLs (expire ~12 min).
// Pass WithLanguage and/or WithEdition to filter to a specific Windows variant.
//
// The arch, ring, and build version embedded in the Build struct must match
// the originating FetchBuilds call — mismatched values cause
// GetExtendedUpdateInfo2 to return empty file locations.
func (s *Service) GetFiles(ctx context.Context, build models.Build, opts ...FileOption) ([]models.File, *resty.Response, error) {
	cfg := &fileConfig{}
	for _, o := range opts {
		o(cfg)
	}

	var fileURLs []wuproto.FileURL

	if cfg.withURLs {
		urls, resp, err := s.client.GetFileURLs(ctx, wuproto.FileURLRequest{
			UpdateID: build.UUID,
			Revision: build.Revision,
			Arch:     wuproto.Arch(build.Arch),
			Ring:     wuproto.Ring(build.Ring),
			Build:    build.Build,
		})
		if err != nil {
			return nil, resp, fmt.Errorf("GetFiles: resolve CDN URLs: %w", err)
		}
		fileURLs = urls
		// Return early with the CDN resp so callers can inspect it.
		files := buildFileList(fileURLs, cfg)
		return files, resp, nil
	}

	// Re-fetch metadata via SyncUpdates — stateless model has no local cache.
	results, resp, err := s.client.FetchUpdates(ctx, wuproto.FetchRequest{
		Arch:  wuproto.Arch(build.Arch),
		Ring:  wuproto.Ring(build.Ring),
		Build: build.Build,
	})
	if err != nil {
		return nil, resp, fmt.Errorf("GetFiles: re-fetch for metadata: %w", err)
	}
	for _, r := range results {
		if r.UpdateID == build.UUID {
			for _, fm := range r.Files {
				fileURLs = append(fileURLs, wuproto.FileURL{
					Name:      fm.Name,
					SHA1:      fm.SHA1,
					SHA256:    fm.SHA256,
					SizeBytes: fm.SizeBytes,
				})
			}
			break
		}
	}

	return buildFileList(fileURLs, cfg), resp, nil
}

// buildFileList converts wuproto.FileURL slice to models.File slice and applies filters.
func buildFileList(fileURLs []wuproto.FileURL, cfg *fileConfig) []models.File {
	files := make([]models.File, 0, len(fileURLs))
	for _, fu := range fileURLs {
		f := models.File{
			Name:      fu.Name,
			SizeBytes: fu.SizeBytes,
			SHA1:      fu.SHA1,
			SHA256:    fu.SHA256,
			FileType:  strings.TrimPrefix(strings.ToLower(filepath.Ext(fu.Name)), "."),
			URL:       fu.URL,
			ExpiresAt: fu.ExpiresAt,
		}
		files = append(files, f)
	}
	return applyFileFilters(files, cfg)
}

// applyFileFilters applies language, edition, and extension filters.
func applyFileFilters(files []models.File, cfg *fileConfig) []models.File {
	if cfg.language == "" && cfg.edition == "" && cfg.extension == "" {
		return files
	}

	result := make([]models.File, 0, len(files))
	for _, f := range files {
		nameLower := strings.ToLower(f.Name)

		// Extension filter.
		if cfg.extension != "" {
			ext := strings.TrimPrefix(cfg.extension, ".")
			if !strings.HasSuffix(nameLower, "."+ext) {
				continue
			}
		}

		// Language filter: include file if it matches the language or is neutral.
		if cfg.language != "" {
			lang := strings.ToLower(cfg.language)
			isNeutral := strings.Contains(nameLower, "_neutral_") ||
				strings.Contains(nameLower, "-neutral") ||
				!strings.Contains(nameLower, "_")
			isMatchingLang := strings.Contains(nameLower, "_"+lang+"_") ||
				strings.Contains(nameLower, "-"+lang+".") ||
				strings.Contains(nameLower, "_"+lang+".")
			if !isNeutral && !isMatchingLang {
				continue
			}
		}

		// Edition filter: include file if it matches the edition or has no edition marker.
		if cfg.edition != "" {
			edLower := strings.ToLower(string(cfg.edition))
			hasEditionMarker := strings.Contains(nameLower, "_professional") ||
				strings.Contains(nameLower, "_core") ||
				strings.Contains(nameLower, "_enterprise") ||
				strings.Contains(nameLower, "_education") ||
				strings.Contains(nameLower, "_server")
			isMatchingEd := strings.Contains(nameLower, "_"+edLower)
			if hasEditionMarker && !isMatchingEd {
				continue
			}
		}

		result = append(result, f)
	}
	return result
}
