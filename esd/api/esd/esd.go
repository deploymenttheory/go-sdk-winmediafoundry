// Package esd resolves Windows installation ESD images from Microsoft's Media
// Creation Tool catalog.
//
// Microsoft publishes a signed cabinet (products.cab) listing every Windows
// installation ESD — by edition, architecture, and language — each with a
// direct, non-expiring CDN download URL and SHA-1. This package fetches that
// cabinet, decompresses it (pure-Go LZX), parses the embedded products.xml, and
// returns the catalog. The chosen ESD can then be downloaded via the Download
// service.
//
// Unlike the SOAP build/file discovery (which surfaces servicing updates), this
// is how a full, bootable install.esd is obtained.
package esd

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/esd/client"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/pkg/cab"
	"resty.dev/v3"
)

// Product selects which Windows product catalog to fetch. The URL is a stable
// Microsoft fwlink redirect to the current products.cab for that product.
type Product struct {
	Name string
	URL  string
}

var (
	// Windows11 is the Windows 11 ESD catalog (default).
	Windows11 = Product{Name: "Windows 11", URL: "https://go.microsoft.com/fwlink/?LinkId=2156292"}
	// Windows10 is the Windows 10 ESD catalog.
	Windows10 = Product{Name: "Windows 10", URL: "https://go.microsoft.com/fwlink/?LinkId=841361"}
)

const productsXMLName = "products.xml"

// Service resolves Windows installation ESD catalogs.
type Service struct {
	client client.Client
}

// New returns a Service backed by the given client transport.
func New(c client.Client) *Service {
	return &Service{client: c}
}

type catalogConfig struct {
	product Product
}

// CatalogOption configures a Catalog call.
type CatalogOption func(*catalogConfig)

// WithProduct selects the product catalog to fetch (defaults to Windows 11).
func WithProduct(p Product) CatalogOption {
	return func(cfg *catalogConfig) { cfg.product = p }
}

// Catalog fetches and parses Microsoft's ESD catalog. The returned
// *resty.Response is from the products.cab fetch.
func (s *Service) Catalog(ctx context.Context, opts ...CatalogOption) (*ESDCatalog, *resty.Response, error) {
	cfg := &catalogConfig{product: Windows11}
	for _, o := range opts {
		o(cfg)
	}

	resp, data, err := s.client.NewRequest(ctx).GetBytes(cfg.product.URL)
	if err != nil {
		return nil, resp, fmt.Errorf("esd.Catalog: fetch products.cab: %w", err)
	}

	xmlData, err := cab.ExtractFile(data, productsXMLName)
	if err != nil {
		return nil, resp, fmt.Errorf("esd.Catalog: extract %s: %w", productsXMLName, err)
	}

	cat, err := parseCatalog(xmlData)
	if err != nil {
		return nil, resp, fmt.Errorf("esd.Catalog: parse %s: %w", productsXMLName, err)
	}
	return cat, resp, nil
}
