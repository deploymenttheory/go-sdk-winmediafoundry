// Package wuproto defines the interface and domain types for the Windows
// Update SOAP protocol layer.
//
// The production implementation lives in internal/wuproto/soap. This package
// is internal to go-sdk-windowsuup and not importable by external modules.
package wuproto

import (
	"context"
	"time"

	"resty.dev/v3"
)

// WindowsUpdateClient abstracts the Windows Update SOAP protocol.
// All methods must be safe for concurrent use.
type WindowsUpdateClient interface {
	// FetchUpdates queries the Windows Update service for builds that match
	// the supplied request parameters. Returns the full set of matching
	// UpdateResult values, each carrying file metadata (hashes + sizes) but
	// not CDN download URLs.
	FetchUpdates(ctx context.Context, req FetchRequest) ([]UpdateResult, *resty.Response, error)

	// GetFileURLs resolves signed CDN download URLs for every file that
	// belongs to the identified update revision. URLs expire in roughly
	// 12 minutes. Callers that only need metadata should avoid this method
	// and rely solely on the FileMetadata embedded in FetchUpdates results.
	GetFileURLs(ctx context.Context, req FileURLRequest) ([]FileURL, *resty.Response, error)
}

// FetchRequest is the input to WindowsUpdateClient.FetchUpdates.
type FetchRequest struct {
	Arch   Arch
	Ring   Ring
	Flight Flight
	// Build filters to a specific build version string (e.g. "26100.4061").
	// An empty string means "latest".
	Build string
	// CheckBuild is the OS version the device claims to be running. Windows
	// Update uses this to determine which upgrades to offer. Set to an old
	// build (e.g. "10.0.9600.0") to receive the current stable release as an
	// upgrade. Defaults to ring-appropriate value when empty.
	CheckBuild string
	// SKU selects a Windows edition by numeric identifier.
	// 0 is treated as the default (48 = Windows 11 Professional).
	SKU  int
	Type BuildType
}

// FileURLRequest is the input to WindowsUpdateClient.GetFileURLs.
type FileURLRequest struct {
	UpdateID string
	Revision int
	// Arch, Ring, and Build must match the context in which the update was
	// discovered (i.e. the values used in the originating SyncUpdates call).
	// EUI2 uses these to build device attributes; mismatched values cause the
	// fe3cr endpoint to return an empty FileLocations list.
	Arch  Arch
	Ring  Ring
	Build string
}

// UpdateResult is a single Windows Update returned by FetchUpdates. It
// carries complete file metadata (hashes and sizes) but NOT CDN download
// URLs — those are fetched on demand via GetFileURLs.
type UpdateResult struct {
	UpdateID     string
	Revision     int
	Title        string
	Build        string // e.g. "26100.4061"
	Arch         Arch
	Files        []FileMetadata
	DiscoveredAt time.Time
}

// FileMetadata describes a single file within a Windows Update set.
// Hashes are always present. URL is empty unless populated by GetFileURLs.
type FileMetadata struct {
	Name      string
	SHA1      string
	SHA256    string
	SizeBytes int64
	Modified  time.Time
}

// FileURL is a resolved CDN download URL returned by GetFileURLs.
type FileURL struct {
	Name      string
	URL       string
	ExpiresAt time.Time
	SizeBytes int64
	SHA1      string
	SHA256    string
}

// Arch represents a CPU architecture supported by Windows Update.
type Arch string

const (
	ArchAMD64 Arch = "amd64"
	ArchX86   Arch = "x86"
	ArchARM64 Arch = "arm64"
	ArchAll   Arch = "all"
)

// Ring represents a Windows Update release channel.
type Ring string

const (
	RingCanary         Ring = "Canary"
	RingExperimental   Ring = "Experimental" // public name for WIF/Dev channel
	RingDev            Ring = "Dev"          // legacy alias → Experimental
	RingBeta           Ring = "Beta"
	RingReleasePreview Ring = "ReleasePreview"
	RingRetail         Ring = "Retail"
	RingMSIT           Ring = "MSIT"
	// Legacy SOAP-name aliases.
	RingWIF Ring = "WIF" // Windows Insider Fast  → Experimental
	RingWIS Ring = "WIS" // Windows Insider Slow  → Beta
	RingRP  Ring = "RP"  // Release Preview       → ReleasePreview
)

// Flight represents the sub-channel within a Ring.
type Flight string

const (
	FlightActive  Flight = "Active"
	FlightSkip    Flight = "Skip"
	FlightCurrent Flight = "Current"
)

// BuildType selects between Production and Test WCOS builds.
type BuildType string

const (
	BuildTypeProduction BuildType = "Production"
	BuildTypeTest       BuildType = "Test"
)
