// Package catalog defines the persistent build catalog domain types and
// storage / event interfaces used throughout the platform.
package catalog

import "time"

// Build is the canonical catalog record for a single Windows Update build.
// It is the central domain type shared by all layers.
type Build struct {
	// UUID is the Windows Update identity key from the SOAP response.
	UUID     string `json:"uuid"`
	Revision int    `json:"revision"`

	// Human-readable metadata.
	Title        string `json:"title"`
	Build        string `json:"build"` // e.g. "26100.4061"
	MajorVersion int    `json:"major_version"`
	MinorVersion int    `json:"minor_version"`
	Arch         string `json:"arch"`
	Ring         string `json:"ring"`
	Flight       string `json:"flight"`
	Branch       string `json:"branch"`
	SKU          int    `json:"sku"`

	// Classification flags (computed at ingest, stored as indexed booleans).
	IsStable     bool `json:"is_stable"`
	IsInsider    bool `json:"is_insider"`
	IsCumulative bool `json:"is_cumulative"`

	// File availability — populated on demand, not stored in builds table.
	Languages []string `json:"languages,omitempty"`
	Editions  []string `json:"editions,omitempty"`

	// Timestamps.
	CreatedAt    time.Time `json:"created_at"`
	DiscoveredAt time.Time `json:"discovered_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// File is a single downloadable file within a build's UUP set.
type File struct {
	// Identity.
	UUID    string `json:"uuid"`    // parent build UUID
	Name    string `json:"name"`
	Lang    string `json:"lang"`    // "" = language-neutral
	Edition string `json:"edition"` // "" = edition-neutral

	// Content addressing.
	SHA1      string `json:"sha1"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`

	// Classification.
	FileType FileType `json:"file_type"`

	// Timestamps.
	ModifiedAt time.Time `json:"modified_at"`
}

// FileType classifies a file within a UUP set.
type FileType string

const (
	FileTypeESD          FileType = "esd"
	FileTypeCAB          FileType = "cab"
	FileTypePSF          FileType = "psf"     // Patch Storage File — excluded from downloads
	FileTypeDifferential FileType = "diff"    // differential update — excluded
	FileTypeEXPRESS      FileType = "express" // express update — excluded
	FileTypeMSIX         FileType = "msix"
	FileTypeUnknown      FileType = "unknown"
)

// BuildDiff is the result of comparing two builds' file sets.
type BuildDiff struct {
	BaseUUID    string `json:"base_uuid"`
	TargetUUID  string `json:"target_uuid"`
	BaseBuild   string `json:"base_build"`
	TargetBuild string `json:"target_build"`

	Added     []FileDiffEntry `json:"added"`
	Removed   []FileDiffEntry `json:"removed"`
	Changed   []FileDiffEntry `json:"changed"`
	Unchanged int             `json:"unchanged_count"`

	GeneratedAt time.Time `json:"generated_at"`
}

// FileDiffEntry is a single file entry within a BuildDiff result.
type FileDiffEntry struct {
	Name       string   `json:"name"`
	BaseFile   *File    `json:"base,omitempty"`   // nil for Added entries
	TargetFile *File    `json:"target,omitempty"` // nil for Removed entries
	ChangeType string   `json:"change_type"`      // "added" | "removed" | "changed"
}

// FeedEntry is a single record in the build change-feed / history log.
type FeedEntry struct {
	ID          int64     `json:"id"`
	EventType   string    `json:"event_type"`
	BuildUUID   string    `json:"build_uuid"`
	BuildTitle  string    `json:"build_title"`
	BuildNumber string    `json:"build_number"`
	Arch        string    `json:"arch"`
	Ring        string    `json:"ring"`
	OccurredAt  time.Time `json:"occurred_at"`
	// Payload is arbitrary JSON metadata for the event (e.g. changed fields).
	Payload []byte `json:"payload,omitempty"`
}

// BuildQuery holds filter and pagination parameters for Store.ListBuilds.
type BuildQuery struct {
	Search     string
	Arch       string
	Ring       string
	StableOnly bool
	Limit      int
	Offset     int
	OrderBy    string // "created_at" | "build_number" | "discovered_at"
	Desc       bool
}

// FeedQuery holds filter and pagination parameters for Store.GetFeed.
type FeedQuery struct {
	Since     time.Time
	EventType string
	Limit     int
	Offset    int
}
