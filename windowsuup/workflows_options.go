package windowsuup

import "github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/constants"

// DownloadBuildOption configures a DownloadBuild or FetchLatestAndDownload call.
type DownloadBuildOption func(*downloadBuildConfig)

type downloadBuildConfig struct {
	language    string
	edition     constants.Edition
	extension   string
	concurrency int
}

// WithDownloadLanguage filters downloaded files to the given BCP-47 language
// tag (e.g. "en-us"). Language-neutral files are always included alongside
// the language-specific files.
func WithDownloadLanguage(lang string) DownloadBuildOption {
	return func(c *downloadBuildConfig) { c.language = lang }
}

// WithDownloadEdition filters downloaded files to the given Windows edition
// (e.g. constants.EditionProfessional). Files with no edition marker are
// always included.
func WithDownloadEdition(ed constants.Edition) DownloadBuildOption {
	return func(c *downloadBuildConfig) { c.edition = ed }
}

// WithDownloadExtension filters downloaded files to the given file extension
// (e.g. ".esd" or ".cab").
func WithDownloadExtension(ext string) DownloadBuildOption {
	return func(c *downloadBuildConfig) { c.extension = ext }
}

// WithDownloadConcurrency sets the number of files downloaded in parallel.
// Defaults to 4 when not set or set to 0.
func WithDownloadConcurrency(n int) DownloadBuildOption {
	return func(c *downloadBuildConfig) { c.concurrency = n }
}
