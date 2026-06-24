// Package softwaredownload resolves and downloads official Windows installation
// ISOs from Microsoft's consumer software-download site — the same flow the
// browser download page performs, reproduced server-side.
//
// The flow has two halves:
//
//   - Scrape (Get/List): fetch the public software-download pages and parse the
//     available product editions (each with the product-edition id Microsoft
//     keys its download API on).
//   - Resolve (GetByID/GetByName): whitelist a session through Microsoft's
//     download "protection" (vlscppe + ov-df), look up the SKU for the desired
//     language, and request the signed, time-limited ISO download link. With
//     WithDownloadDir the resolved ISO is streamed straight to disk.
//
// It is structured like the windowsuup/esd service clients (transport, options,
// models, mocks) but is fully self-contained.
package softwaredownload

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/client"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/constants"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/shared/models"
	"resty.dev/v3"
)

// Microsoft consumer download-flow constants. These mirror the values the
// browser download page uses; they are stable identifiers, not secrets.
const (
	orgID      = "y6jn8c31"
	profileID  = "606624d44113"
	instanceID = "560dc9f3-1aa5-4a2f-b63c-9e18f8d0e175"

	vlscppeTagsURL = "https://vlscppe.microsoft.com/tags"
	ovdfMdtJSURL   = "https://ov-df.microsoft.com/mdt.js"
	ovdfURL        = "https://ov-df.microsoft.com/"
	skuInfoURL     = "https://www.microsoft.com/software-download-connector/api/getskuinformationbyproductedition"
	linksURL       = "https://www.microsoft.com/software-download-connector/api/GetProductDownloadLinksBySku"

	// connectorReferer is required by GetProductDownloadLinksBySku — the
	// connector denies the request without it.
	connectorReferer = "https://www.microsoft.com/software-download/windows11"

	// browserUserAgent is sent on every request: Microsoft's download flow
	// (page scrape and connector API) behaves differently for non-browser agents.
	browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

	// defaultLocale is the page/connector locale.
	defaultLocale = "en-US"
	// defaultLanguage is the ISO language resolution targets by default.
	defaultLanguage = "English (United States)"
)

// Page is a Microsoft software-download page to scrape. Microsoft splits Windows
// 11 ISO downloads across one page per architecture, each exposing a distinct
// product-edition id.
type Page struct {
	// Name is a human label for the page.
	Name string
	// Path is the software-download path appended after the locale, e.g.
	// "software-download/windows11arm64".
	Path string
	// Arch is the architecture this page serves.
	Arch constants.Arch
}

var (
	// Windows11x64 is the x64 Windows 11 multi-edition ISO download page.
	Windows11x64 = Page{Name: "Windows 11 (x64)", Path: "software-download/windows11", Arch: constants.ArchX64}
	// Windows11ARM64 is the Arm64 Windows 11 multi-edition ISO download page.
	Windows11ARM64 = Page{Name: "Windows 11 (Arm64)", Path: "software-download/windows11arm64", Arch: constants.ArchARM64}
)

// defaultPages is the page set scraped when WithPages is not supplied.
func defaultPages() []Page { return []Page{Windows11x64, Windows11ARM64} }

// url builds the absolute page URL for the given locale.
func (p Page) url(locale string) string {
	return fmt.Sprintf("https://www.microsoft.com/%s/%s", locale, p.Path)
}

// Service resolves and downloads official Windows installation ISOs.
type Service struct {
	client client.Client
}

// New returns a Service backed by the given client transport.
func New(c client.Client) *Service {
	return &Service{client: c}
}

// Get performs the scrape only: it fetches the selected software-download pages
// and returns the product editions they advertise, each with the product-edition
// id used by GetByID. No session is established and no download link is resolved.
//
// By default both the Windows 11 x64 and Arm64 pages are scraped; narrow with
// WithArch or WithPages. The returned *resty.Response is from the last page
// fetched.
func (s *Service) Get(ctx context.Context, opts ...Option) (*models.Catalog, *resty.Response, error) {
	cfg := applyOptions(opts)

	pages := cfg.pages
	if len(pages) == 0 {
		pages = defaultPages()
	}

	cat := &models.Catalog{}
	var lastResp *resty.Response
	for _, page := range pages {
		if cfg.arch != "" && page.Arch != "" && page.Arch != cfg.arch {
			continue
		}

		pageURL := page.url(cfg.locale)
		resp, data, err := s.client.NewRequest(ctx).
			SetHeader("User-Agent", browserUserAgent).
			GetBytes(pageURL)
		lastResp = resp
		if err != nil {
			return nil, resp, fmt.Errorf("softwaredownload.Get: fetch %s: %w", page.Name, err)
		}

		for _, prod := range parseEditions(data, page, pageURL) {
			if cfg.arch != "" && prod.Arch != "" && prod.Arch != cfg.arch {
				continue
			}
			cat.Products = append(cat.Products, prod)
		}
	}

	return cat, lastResp, nil
}

// List is a convenience wrapper over Get returning the flat slice of products
// (after applying the same WithArch/WithPages filters).
func (s *Service) List(ctx context.Context, opts ...Option) ([]models.Product, *resty.Response, error) {
	cat, resp, err := s.Get(ctx, opts...)
	if err != nil {
		return nil, resp, err
	}
	return cat.Products, resp, nil
}
