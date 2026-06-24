package softwaredownload

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/constants"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/shared/models"
	"resty.dev/v3"
)

// skuInfoMaxAttempts bounds the retries on the SKU lookup. Microsoft's connector
// occasionally returns an empty/transient error on the first call right after a
// fresh session is whitelisted, so a couple of attempts are needed.
const skuInfoMaxAttempts = 3

// GetByID resolves the signed ISO download link for a specific product-edition
// id (as returned by Get/List, e.g. "3324" for Windows 11 Arm64). It whitelists
// a fresh session, looks up the SKU for the requested language (WithLanguage,
// default "English (United States)"), and returns the download link.
//
// With WithDownloadDir the resolved ISO is streamed to disk and
// DownloadLink.LocalPath is set. The returned *resty.Response is from the
// download-links request.
func (s *Service) GetByID(ctx context.Context, editionID string, opts ...Option) (*models.DownloadLink, *resty.Response, error) {
	if strings.TrimSpace(editionID) == "" {
		return nil, nil, fmt.Errorf("softwaredownload.GetByID: editionID is required")
	}
	cfg := applyOptions(opts)
	product := models.Product{EditionID: editionID, Arch: cfg.arch}
	return s.resolve(ctx, product, cfg)
}

// GetByName resolves the signed ISO download link for the first product edition
// whose page label contains name (case-insensitive) — e.g. "Arm64" selects
// "Windows 11 (multi-edition ISO for Arm64)". It scrapes to find the edition,
// then resolves it like GetByID. WithArch narrows the scrape first.
func (s *Service) GetByName(ctx context.Context, name string, opts ...Option) (*models.DownloadLink, *resty.Response, error) {
	if strings.TrimSpace(name) == "" {
		return nil, nil, fmt.Errorf("softwaredownload.GetByName: name is required")
	}
	cfg := applyOptions(opts)

	cat, resp, err := s.Get(ctx, opts...)
	if err != nil {
		return nil, resp, err
	}

	product, ok := findProduct(cat.Products, name)
	if !ok {
		return nil, resp, fmt.Errorf("softwaredownload.GetByName: no product edition matching %q (have %s)",
			name, strings.Join(productNames(cat.Products), ", "))
	}
	return s.resolve(ctx, product, cfg)
}

// resolve performs the full session → SKU → links flow for a product and, when
// WithDownloadDir is set, downloads the resolved ISO.
func (s *Service) resolve(ctx context.Context, product models.Product, cfg *config) (*models.DownloadLink, *resty.Response, error) {
	sessionID, err := newSessionID()
	if err != nil {
		return nil, nil, fmt.Errorf("softwaredownload.resolve: new session id: %w", err)
	}

	if err := s.whitelistSession(ctx, sessionID); err != nil {
		return nil, nil, fmt.Errorf("softwaredownload.resolve: whitelist session: %w", err)
	}

	lang, err := s.resolveLanguage(ctx, product.EditionID, sessionID, cfg.locale, cfg.language)
	if err != nil {
		return nil, nil, err
	}

	link, resp, err := s.resolveLink(ctx, product, lang, sessionID, cfg.locale)
	if err != nil {
		return nil, resp, err
	}

	if cfg.arch != "" && link.Arch != "" && link.Arch != cfg.arch {
		return nil, resp, fmt.Errorf("softwaredownload.resolve: resolved %s media but WithArch requested %s",
			link.Arch, cfg.arch)
	}

	if cfg.downloadDir != "" {
		dest, _, derr := s.downloadOne(ctx, *link, cfg.downloadDir, cfg)
		if derr != nil {
			return link, resp, derr
		}
		link.LocalPath = dest
	}

	return link, resp, nil
}

// whitelistSession runs Microsoft's download "protection" handshake. Tagging the
// session through vlscppe is the part the connector actually requires; the ov-df
// mdt.js challenge/response is performed best-effort to mirror the browser and
// guard against Microsoft re-tightening the gate (its absence is currently
// tolerated, and the mdt.js token set has changed over time).
func (s *Service) whitelistSession(ctx context.Context, sessionID string) error {
	// 1) vlscppe session tag (required).
	if _, err := s.client.NewRequest(ctx).
		SetHeader("User-Agent", browserUserAgent).
		SetQueryParam("org_id", orgID).
		SetQueryParam("session_id", sessionID).
		Get(vlscppeTagsURL); err != nil {
		return fmt.Errorf("vlscppe tag: %w", err)
	}

	// 2) ov-df challenge/response (best-effort).
	s.completeOVDF(ctx, sessionID)
	return nil
}

// completeOVDF performs the ov-df mdt.js challenge and echoes the token back. It
// never returns an error: the connector currently accepts a vlscppe-tagged
// session without it, so any failure here must not block resolution.
func (s *Service) completeOVDF(ctx context.Context, sessionID string) {
	_, mdt, err := s.client.NewRequest(ctx).
		SetHeader("User-Agent", browserUserAgent).
		SetQueryParam("instanceId", instanceID).
		SetQueryParam("PageId", "si").
		SetQueryParam("session_id", sessionID).
		GetBytes(ovdfMdtJSURL)
	if err != nil {
		return
	}
	w, rticks := parseMDT(mdt)
	if w == "" {
		return
	}

	req := s.client.NewRequest(ctx).
		SetHeader("User-Agent", browserUserAgent).
		SetQueryParam("session_id", sessionID).
		SetQueryParam("CustomerId", instanceID).
		SetQueryParam("PageId", "si").
		SetQueryParam("w", w).
		SetQueryParam("mdt", fmt.Sprintf("%d", time.Now().UnixMilli()))
	if rticks != "" {
		req = req.SetQueryParam("rticks", rticks)
	}
	_, _ = req.Get(ovdfURL)
}

// resolveLanguage looks up the SKU for the desired language on a product edition.
func (s *Service) resolveLanguage(ctx context.Context, editionID, sessionID, locale, language string) (models.Language, error) {
	var lastErr error
	for attempt := range skuInfoMaxAttempts {
		if attempt > 0 {
			time.Sleep(2 * time.Second)
		}

		// The connector responds with content-type text/plain, so the body is
		// parsed explicitly rather than via resty's content-type-gated SetResult.
		_, body, err := s.client.NewRequest(ctx).
			SetHeader("User-Agent", browserUserAgent).
			SetQueryParam("profile", profileID).
			SetQueryParam("productEditionId", editionID).
			SetQueryParam("SKU", "undefined").
			SetQueryParam("friendlyFileName", "undefined").
			SetQueryParam("Locale", locale).
			SetQueryParam("sessionID", sessionID).
			GetBytes(skuInfoURL)
		if err != nil {
			lastErr = err
			continue
		}
		var info skuInfoResponse
		if err := json.Unmarshal(body, &info); err != nil {
			lastErr = fmt.Errorf("decode SKU response: %w", err)
			continue
		}
		if msg := firstError(info.Errors); msg != "" {
			lastErr = fmt.Errorf("connector: %s", msg)
			continue
		}
		if len(info.Skus) == 0 {
			lastErr = fmt.Errorf("no SKUs returned for edition %s", editionID)
			continue
		}

		sku, ok := selectSKU(info, language)
		if !ok {
			return models.Language{}, fmt.Errorf("softwaredownload: language %q not available for edition %s (have %s)",
				language, editionID, strings.Join(skuLanguages(info), ", "))
		}
		return models.Language{
			Name:          sku.Language,
			LocalizedName: sku.LocalizedLanguage,
			SKUID:         sku.ID,
		}, nil
	}
	return models.Language{}, fmt.Errorf("softwaredownload: resolve language for edition %s: %w", editionID, lastErr)
}

// resolveLink requests the signed download link for a resolved SKU.
func (s *Service) resolveLink(ctx context.Context, product models.Product, lang models.Language, sessionID, locale string) (*models.DownloadLink, *resty.Response, error) {
	resp, body, err := s.client.NewRequest(ctx).
		SetHeader("User-Agent", browserUserAgent).
		SetHeader("Referer", connectorReferer).
		SetQueryParam("profile", profileID).
		SetQueryParam("productEditionId", "undefined").
		SetQueryParam("SKU", lang.SKUID).
		SetQueryParam("friendlyFileName", "undefined").
		SetQueryParam("Locale", locale).
		SetQueryParam("sessionID", sessionID).
		GetBytes(linksURL)
	if err != nil {
		return nil, resp, fmt.Errorf("softwaredownload: fetch download links: %w", err)
	}
	var links linksResponse
	if err := json.Unmarshal(body, &links); err != nil {
		return nil, resp, fmt.Errorf("softwaredownload: decode download-links response: %w", err)
	}
	if msg := firstError(links.Errors); msg != "" {
		return nil, resp, fmt.Errorf("softwaredownload: connector rejected link request: %s", msg)
	}
	if len(links.ProductDownloadOptions) == 0 {
		return nil, resp, fmt.Errorf("softwaredownload: no download links returned for SKU %s", lang.SKUID)
	}

	opt := links.ProductDownloadOptions[0]
	fileName := fileNameFromURL(opt.Uri)
	arch := product.Arch
	if a := constants.ArchFromToken(fileName); a != "" {
		arch = a // the filename's arch token is authoritative
	}

	return &models.DownloadLink{
		Product:   product,
		Language:  lang,
		Arch:      arch,
		FileName:  fileName,
		URL:       opt.Uri,
		ExpiresAt: expiryFromURL(opt.Uri),
	}, resp, nil
}

// selectSKU picks the SKU matching language, preferring an exact (case-insensitive)
// match on either the locale name or the localized name, then a substring match.
func selectSKU(info skuInfoResponse, language string) (skuEntry, bool) {
	want := strings.TrimSpace(language)
	for _, s := range info.Skus {
		if strings.EqualFold(s.Language, want) || strings.EqualFold(s.LocalizedLanguage, want) {
			return s, true
		}
	}
	for _, s := range info.Skus {
		if strings.Contains(strings.ToLower(s.Language), strings.ToLower(want)) ||
			strings.Contains(strings.ToLower(s.LocalizedLanguage), strings.ToLower(want)) {
			return s, true
		}
	}
	return skuEntry{}, false
}

// findProduct returns the first product whose Name contains name (case-insensitive).
func findProduct(products []models.Product, name string) (models.Product, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	for _, p := range products {
		if strings.Contains(strings.ToLower(p.Name), want) {
			return p, true
		}
	}
	return models.Product{}, false
}

func productNames(products []models.Product) []string {
	out := make([]string, 0, len(products))
	for _, p := range products {
		out = append(out, p.Name)
	}
	return out
}

func skuLanguages(info skuInfoResponse) []string {
	out := make([]string, 0, len(info.Skus))
	for _, s := range info.Skus {
		out = append(out, s.LocalizedLanguage)
	}
	return out
}
