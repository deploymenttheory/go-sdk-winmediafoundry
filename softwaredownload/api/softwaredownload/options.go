package softwaredownload

import (
	"io"
	"time"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/pkg/progress_counter"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/constants"
)

// ProgressCallback is called periodically during a download with the current
// byte count, total expected size (0 when unknown), and elapsed time. It matches
// progress_counter.Bar.Callback so callers can pass that directly.
type ProgressCallback func(fileName string, written, total int64, elapsed time.Duration)

// Option configures a softwaredownload operation. The same option type is shared
// across the scrape (Get/List), resolution (GetByID/GetByName), and download so
// that, for example, GetByName can both pick the right edition and stream the
// ISO from a single call.
type Option func(*config)

// config holds resolved option values for an operation.
type config struct {
	// scrape
	pages  []Page
	arch   constants.Arch
	locale string

	// resolution
	language string

	// download
	downloadDir      string
	progressCallback ProgressCallback
}

func defaultConfig() *config {
	return &config{
		locale:   defaultLocale,
		language: defaultLanguage,
	}
}

func applyOptions(opts []Option) *config {
	cfg := defaultConfig()
	for _, o := range opts {
		o(cfg)
	}
	return cfg
}

// WithPages overrides the set of software-download pages to scrape (default:
// the Windows 11 x64 and Arm64 pages). Used by Get and List.
func WithPages(pages ...Page) Option {
	return func(c *config) {
		if len(pages) > 0 {
			c.pages = pages
		}
	}
}

// WithArch restricts a scrape to a single architecture and selects the
// architecture when resolving a download link. Empty (default) means all
// architectures.
func WithArch(arch constants.Arch) Option {
	return func(c *config) { c.arch = arch }
}

// WithLocale sets the page locale and the connector Locale parameter
// (default "en-US"). This affects which localized product names are returned;
// it is independent of the ISO language (see WithLanguage).
func WithLocale(locale string) Option {
	return func(c *config) {
		if locale != "" {
			c.locale = locale
		}
	}
}

// WithLanguage selects the ISO language to resolve, matched case-insensitively
// against either the locale tag (e.g. "en-US") or the localized name
// (e.g. "English (United States)"). Default: "English (United States)".
func WithLanguage(language string) Option {
	return func(c *config) {
		if language != "" {
			c.language = language
		}
	}
}

// WithDownloadDir makes GetByID/GetByName stream the resolved ISO into dir
// (created if needed) and populate DownloadLink.LocalPath. Without it, those
// calls only resolve the signed URL.
func WithDownloadDir(dir string) Option {
	return func(c *config) { c.downloadDir = dir }
}

// WithProgress writes a terminal progress bar to w during a download.
// Pass nil to write to os.Stderr.
func WithProgress(w io.Writer) Option {
	return func(c *config) {
		bar := progress_counter.New(w)
		c.progressCallback = bar.Callback
	}
}

// WithProgressCallback sets a custom progress callback. This is the lower-level
// alternative to WithProgress.
func WithProgressCallback(fn ProgressCallback) Option {
	return func(c *config) { c.progressCallback = fn }
}
