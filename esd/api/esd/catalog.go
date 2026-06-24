package esd

import (
	"strconv"
	"strings"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/esd/shared/models"
)

// ESDImage is a single Windows installation ESD listed in Microsoft's Media
// Creation Tool catalog (products.xml). Unlike the SOAP file path, URL is a
// direct, non-expiring Microsoft CDN download link.
type ESDImage struct {
	// FileName is the ESD file name, e.g.
	// "26100.4349.250607-1500.ge_release_svc_refresh_CLIENTCONSUMER_RET_x64FRE_en-us.esd".
	FileName string
	// Edition is the Windows edition, e.g. "Professional", "Enterprise", "Core".
	Edition string
	// Architecture is "x64" or "ARM64".
	Architecture string
	// LanguageCode is the BCP-47 language tag, e.g. "en-us".
	LanguageCode string
	// Language is the human-readable language, e.g. "English (United States)".
	Language string
	// SizeBytes is the download size in bytes.
	SizeBytes int64
	// SHA1 is the lower-case hex SHA-1 of the ESD, as published in the catalog.
	SHA1 string
	// URL is the direct Microsoft CDN download URL (dl.delivery.mp.microsoft.com).
	URL string
}

// AsFile adapts an ESDImage to a models.File so it can be fetched via the
// Download service. The catalog's hex SHA-1 is not copied into File.SHA1 (which
// the SDK treats as base64); verify ESD downloads against ESDImage.SHA1 directly.
func (e ESDImage) AsFile() models.File {
	return models.File{
		Name:      e.FileName,
		SizeBytes: e.SizeBytes,
		FileType:  "esd",
		URL:       e.URL,
	}
}

// BuildMajor returns the leading build number of the ESD filename — e.g.
// "26100.4349.250607-1500.ge_release..." yields 26100 — or 0 if the filename
// does not begin with a number. This is the value a Windows 11 feature release
// name resolves to (see constants.ReleaseBuild).
func (e ESDImage) BuildMajor() int {
	dot := strings.IndexByte(e.FileName, '.')
	if dot <= 0 {
		return 0
	}
	n, err := strconv.Atoi(e.FileName[:dot])
	if err != nil {
		return 0
	}
	return n
}

// archFromFileName extracts the architecture encoded in an ESD filename. MCT
// ESD names carry an architecture token — "A64FRE" for ARM64, "X64FRE" for x64
// (e.g. "...CLIENTCONSUMER_RET_A64FRE_en-us.esd"). It returns "ARM64", "x64",
// or "" when no token is present. This token is authoritative: it lets callers
// detect a row whose Architecture field disagrees with its own filename.
func archFromFileName(fileName string) string {
	up := strings.ToUpper(fileName)
	switch {
	case strings.Contains(up, "A64FRE"), strings.Contains(up, "ARM64"):
		return "ARM64"
	case strings.Contains(up, "X64FRE"), strings.Contains(up, "AMD64"):
		return "x64"
	default:
		return ""
	}
}

// IsARM64 reports whether the image is ARM64 Windows media. When the filename
// carries an architecture token it is treated as authoritative (so a row
// mislabeled Architecture="ARM64" but named "...X64FRE..." is correctly
// rejected); otherwise the Architecture field is trusted.
func (e ESDImage) IsARM64() bool {
	if a := archFromFileName(e.FileName); a != "" {
		return a == "ARM64"
	}
	return strings.EqualFold(e.Architecture, "ARM64")
}

// ESDCatalog is the parsed Media Creation Tool ESD catalog.
type ESDCatalog struct {
	// Images holds every ESD entry in the catalog.
	Images []ESDImage
}

// Filter returns the images matching the given edition, architecture, and
// language. Empty arguments match any value; matching is case-insensitive.
func (c *ESDCatalog) Filter(edition, architecture, languageCode string) []ESDImage {
	var out []ESDImage
	for _, img := range c.Images {
		if edition != "" && !strings.EqualFold(img.Edition, edition) {
			continue
		}
		if architecture != "" && !strings.EqualFold(img.Architecture, architecture) {
			continue
		}
		if languageCode != "" && !strings.EqualFold(img.LanguageCode, languageCode) {
			continue
		}
		out = append(out, img)
	}
	return out
}

// FilterBuildMajor returns the images whose filename build-major equals build,
// after applying the same edition/architecture/language filters as Filter. A
// build of 0 disables the build filter (equivalent to Filter). Empty
// edition/architecture/language arguments match any value.
func (c *ESDCatalog) FilterBuildMajor(build int, edition, architecture, languageCode string) []ESDImage {
	base := c.Filter(edition, architecture, languageCode)
	if build == 0 {
		return base
	}
	out := make([]ESDImage, 0, len(base))
	for _, img := range base {
		if img.BuildMajor() == build {
			out = append(out, img)
		}
	}
	return out
}

// FilterARM64BuildMajor returns the ARM64 images whose filename build-major
// equals build, after applying the edition/language filters. Unlike
// FilterBuildMajor it cannot be asked for the wrong architecture: ARM64
// selection is enforced via IsARM64, which also drops rows whose filename token
// contradicts their Architecture field. A build of 0 disables the build filter.
func (c *ESDCatalog) FilterARM64BuildMajor(build int, edition, languageCode string) []ESDImage {
	out := make([]ESDImage, 0)
	for _, img := range c.FilterBuildMajor(build, edition, "", languageCode) {
		if img.IsARM64() {
			out = append(out, img)
		}
	}
	return out
}

// BuildMajors returns the distinct build-major numbers present in the catalog.
func (c *ESDCatalog) BuildMajors() []int {
	seen := make(map[int]struct{})
	var out []int
	for _, img := range c.Images {
		b := img.BuildMajor()
		if _, ok := seen[b]; ok {
			continue
		}
		seen[b] = struct{}{}
		out = append(out, b)
	}
	return out
}

// Editions returns the distinct edition names present in the catalog.
func (c *ESDCatalog) Editions() []string {
	return c.distinct(func(i ESDImage) string { return i.Edition })
}

// Architectures returns the distinct architectures present in the catalog.
func (c *ESDCatalog) Architectures() []string {
	return c.distinct(func(i ESDImage) string { return i.Architecture })
}

// Languages returns the distinct language codes present in the catalog.
func (c *ESDCatalog) Languages() []string {
	return c.distinct(func(i ESDImage) string { return i.LanguageCode })
}

func (c *ESDCatalog) distinct(key func(ESDImage) string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, img := range c.Images {
		k := key(img)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}
