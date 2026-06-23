package files

import (
	"strings"

	"github.com/deploymenttheory/winmediafoundry/windowsuup/constants"
)

// FileOption configures a GetFiles call.
type FileOption func(*fileConfig)

type fileConfig struct {
	withURLs  bool
	language  string
	edition   constants.Edition
	extension string
}

// WithCDNURLs resolves live Microsoft CDN download URLs for each file.
// URLs expire approximately 12 minutes after resolution — download promptly
// or call GetFiles again to re-resolve.
func WithCDNURLs() FileOption {
	return func(c *fileConfig) { c.withURLs = true }
}

// WithLanguage filters files to those matching the given BCP-47 language tag
// (e.g. "en-us", "de-de"). Files with "neutral" in their name are always
// included alongside the language-specific files.
func WithLanguage(lang string) FileOption {
	return func(c *fileConfig) { c.language = strings.ToLower(lang) }
}

// WithEdition filters files to those matching the given Windows edition.
// Uses filename substring matching (e.g. EditionProfessional matches
// files containing "PROFESSIONAL" in their name).
func WithEdition(ed constants.Edition) FileOption {
	return func(c *fileConfig) { c.edition = ed }
}

// WithExtension filters files to those with the given extension (e.g. ".esd", ".cab").
func WithExtension(ext string) FileOption {
	return func(c *fileConfig) { c.extension = strings.ToLower(ext) }
}
