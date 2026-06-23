// Package files provides Windows Update file resolution operations.
package files

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/deploymenttheory/winmediafoundry/internal/wuproto"
	"github.com/deploymenttheory/winmediafoundry/internal/wuproto/soap"
	"github.com/deploymenttheory/winmediafoundry/windowsuup/client"
	"github.com/deploymenttheory/winmediafoundry/windowsuup/constants"
	"github.com/deploymenttheory/winmediafoundry/windowsuup/shared/models"
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

	if cfg.withURLs {
		return s.resolveFileURLs(ctx, build, cfg)
	}
	return s.fetchFileMetadata(ctx, build, cfg)
}

// resolveFileURLs calls GetExtendedUpdateInfo2 to get signed CDN download URLs.
// URLs expire approximately 12 minutes after resolution.
func (s *Service) resolveFileURLs(ctx context.Context, build models.Build, cfg *fileConfig) ([]models.File, *resty.Response, error) {
	arch := string(build.Arch)
	ring := string(build.Ring)
	deviceAttrs := soap.BuildDeviceAttributes(arch, ring, build.Build, "", build.SKU, "")

	// Cookie-aware retry: invalidate and retry once on cookie errors.
	const maxCookieRetries = 1
	for attempt := 0; attempt <= maxCookieRetries; attempt++ {
		_, _, devToken, err := s.client.AcquireWUCookie(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("GetFiles: acquire WU cookie: %w", err)
		}

		envelope := soap.BuildGetEUI2Envelope(time.Now(), devToken, build.UUID, build.Revision, deviceAttrs)

		resp, err := s.client.NewRequest(ctx).
			SetHeader("Content-Type", constants.ApplicationSOAPXML).
			SetHeader("SOAPAction", soap.GetEUI2Action).
			SetBody(envelope).
			Post(soap.ClientSecuredEndpoint)
		if err != nil {
			if attempt < maxCookieRetries && soap.IsCookieError(err.Error()) {
				s.client.InvalidateWUCookie()
				continue
			}
			return nil, resp, fmt.Errorf("GetFiles: GetExtendedUpdateInfo2: %w", err)
		}

		fileURLs, parseErr := soap.ParseFileURLs(resp.Bytes())
		if parseErr != nil {
			return nil, resp, fmt.Errorf("GetFiles: parse EUI2 response: %w", parseErr)
		}

		return buildFileList(fileURLs, cfg), resp, nil
	}

	return nil, nil, fmt.Errorf("GetFiles: unexpected retry loop exit")
}

// fetchFileMetadata re-fetches build metadata via SyncUpdates.
// Used when CDN URLs are not needed — avoids the more expensive EUI2 call.
func (s *Service) fetchFileMetadata(ctx context.Context, build models.Build, cfg *fileConfig) ([]models.File, *resty.Response, error) {
	arch := string(build.Arch)
	ring := string(build.Ring)
	deviceAttrs := soap.BuildDeviceAttributes(arch, ring, build.Build, "", build.SKU, "")
	products := soap.BuildProductsString(arch, ring, build.Build, build.SKU)

	// Cookie-aware retry: invalidate and retry once on cookie errors.
	const maxCookieRetries = 1
	for attempt := 0; attempt <= maxCookieRetries; attempt++ {
		encData, expiry, devToken, err := s.client.AcquireWUCookie(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("GetFiles: acquire WU cookie: %w", err)
		}

		envelope := soap.BuildSyncUpdatesEnvelope(
			time.Now(), devToken, encData, expiry,
			deviceAttrs, soap.CallerAttrs, products,
			true, // syncCurrentOnly: we know the exact build
		)

		resp, err := s.client.NewRequest(ctx).
			SetHeader("Content-Type", constants.ApplicationSOAPXML).
			SetHeader("SOAPAction", soap.SyncUpdatesAction).
			SetBody(envelope).
			Post(soap.ClientEndpoint)
		if err != nil {
			if attempt < maxCookieRetries && soap.IsCookieError(err.Error()) {
				s.client.InvalidateWUCookie()
				continue
			}
			return nil, resp, fmt.Errorf("GetFiles: SyncUpdates: %w", err)
		}

		results, parseErr := soap.ParseSyncUpdatesResponse(resp.Bytes())
		if parseErr != nil {
			return nil, resp, fmt.Errorf("GetFiles: parse SyncUpdates response: %w", parseErr)
		}

		var fileURLs []wuproto.FileURL
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

	return nil, nil, fmt.Errorf("GetFiles: unexpected retry loop exit")
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
