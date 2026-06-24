// Package models defines the shared types returned by the softwaredownload
// service operations.
package models

import (
	"time"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/constants"
)

// Product is a downloadable Windows product edition discovered by scraping a
// Microsoft software-download page. It is the unit returned by the scrape
// (Get/List) and the input that resolution (GetByID/GetByName) turns into one
// or more DownloadLinks.
type Product struct {
	// EditionID is Microsoft's product-edition id, e.g. "3324" for
	// "Windows 11 (multi-edition ISO for Arm64)". It is the value passed as
	// productEditionId to the download-connector SKU lookup.
	EditionID string
	// Name is the human-readable edition label from the page, e.g.
	// "Windows 11 (multi-edition ISO for Arm64)".
	Name string
	// Arch is the architecture inferred from the page/name (x64 or ARM64).
	// Empty when it cannot be determined from the label.
	Arch constants.Arch
	// PageURL is the software-download page the product was scraped from.
	PageURL string
}

// Catalog is the result of a scrape: the set of downloadable products across
// the requested Windows software-download pages.
type Catalog struct {
	// Products holds every product-edition entry discovered.
	Products []Product
}

// Language is a downloadable language offered for a product edition, as returned
// by the download-connector SKU lookup.
type Language struct {
	// Name is the locale tag the connector keys on, e.g. "en-US".
	Name string
	// LocalizedName is the human-readable language, e.g. "English (United States)".
	LocalizedName string
	// SKUID is the connector SKU id used to resolve download links for this
	// language.
	SKUID string
}

// DownloadLink is a resolved, time-limited ISO download for a specific product
// edition and language. Microsoft signs the URL with an expiry (~24 hours).
type DownloadLink struct {
	// Product is the edition this link belongs to.
	Product Product
	// Language is the language the link was resolved for.
	Language Language
	// Arch is the architecture of the linked media (derived from the URL/name,
	// which is authoritative).
	Arch constants.Arch
	// FileName is the ISO file name parsed from the URL, e.g.
	// "Win11_25H2_English_Arm64_v2.iso".
	FileName string
	// URL is the signed Microsoft CDN download URL
	// (software.download.prss.microsoft.com / software-static.download.prss...).
	URL string
	// SizeBytes is the download size when known (from a HEAD/Content-Length),
	// otherwise 0.
	SizeBytes int64
	// ExpiresAt is the link expiry parsed from the URL signature, or the zero
	// value when it cannot be determined. Microsoft links are valid ~24 hours.
	ExpiresAt time.Time
	// LocalPath is the on-disk path after a successful download, otherwise "".
	LocalPath string
}
