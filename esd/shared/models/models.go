// Package models defines the shared types returned by all Windows Update SDK
// operations.
package models

import (
	"time"

	"github.com/deploymenttheory/winmediafoundry/esd/constants"
)

// Build represents a Windows Update discovered from Microsoft's update service.
type Build struct {
	// UUID is the Windows Update identity key (a standard UUID string).
	UUID     string
	Revision int
	// Title is the human-readable update title (may be empty for some builds).
	Title string
	// Build is the full version string, e.g. "10.0.26300.8289".
	Build        string
	Arch         constants.Arch
	Ring         constants.Ring
	Flight       string
	Branch       string
	SKU          int
	IsStable     bool
	IsInsider    bool
	DiscoveredAt time.Time
}

// File represents a single file within a Windows Update.
// URL and ExpiresAt are only populated when GetFiles is called with WithCDNURLs.
type File struct {
	Name      string
	SizeBytes int64
	SHA1      string
	SHA256    string
	// FileType is the lower-case extension without the dot: "esd", "cab", etc.
	FileType   string
	ModifiedAt time.Time
	// URL is the live Microsoft CDN download URL. Empty unless WithCDNURLs was used.
	URL string
	// ExpiresAt is the CDN URL expiry time (~12 minutes from resolution).
	// Zero when URL is empty.
	ExpiresAt time.Time
}

// BuildDiff is the result of comparing the file sets of two builds.
type BuildDiff struct {
	BaseUUID    string
	TargetUUID  string
	BaseBuild   string
	TargetBuild string
	// Added contains files present in target but not in base.
	Added []File
	// Removed contains files present in base but not in target.
	Removed []File
	// Changed contains files present in both builds with different hashes.
	Changed []FileDiff
	// Unchanged is the count of files identical in both builds.
	Unchanged int
}

// FileDiff records a single changed file between two builds.
type FileDiff struct {
	Name       string
	BaseFile   File
	TargetFile File
}
