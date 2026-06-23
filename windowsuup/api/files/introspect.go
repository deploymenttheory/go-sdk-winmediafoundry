package files

import (
	"regexp"
	"slices"
	"strings"

	"github.com/deploymenttheory/winmediafoundry/windowsuup/constants"
	"github.com/deploymenttheory/winmediafoundry/windowsuup/shared/models"
)

// reLang matches a BCP-47 language code embedded in a Windows Update filename.
// Patterns matched: _en-us_, _en-us., -en-us., -en-us (end of string).
var reLang = regexp.MustCompile(`(?:_|-)([a-z]{2,3}-[a-z]{2,4})(?:[_.]|$)`)

// knownEditions is the ordered set of Edition constants checked by ListEditions.
// EditionNeutral (empty string) is intentionally excluded — it would match every filename.
var knownEditions = []constants.Edition{
	constants.EditionProfessional,
	constants.EditionProfessionalN,
	constants.EditionHome,
	constants.EditionHomeN,
	constants.EditionEnterprise,
	constants.EditionEnterpriseN,
	constants.EditionEducation,
	constants.EditionEducationN,
	constants.EditionProWorkstation,
	constants.EditionServerStandard,
	constants.EditionServerDatacenter,
}

// ListLanguages returns the distinct BCP-47 language codes present in the
// given file list, extracted from filename patterns such as _en-us_ or -en-us.
// Results are sorted and deduplicated. Neutral files contribute no language code.
func ListLanguages(files []models.File) []string {
	seen := make(map[string]struct{})
	for _, f := range files {
		nameLower := strings.ToLower(f.Name)
		for _, m := range reLang.FindAllStringSubmatch(nameLower, -1) {
			seen[m[1]] = struct{}{}
		}
	}

	langs := make([]string, 0, len(seen))
	for lang := range seen {
		langs = append(langs, lang)
	}
	slices.Sort(langs)
	return langs
}

// ListEditions returns the distinct Windows edition constants present in the
// given file list, extracted by matching known edition tokens in filenames.
// Files with no edition marker contribute nothing.
func ListEditions(files []models.File) []constants.Edition {
	seen := make(map[constants.Edition]struct{})
	for _, f := range files {
		nameLower := strings.ToLower(f.Name)
		for _, ed := range knownEditions {
			if strings.Contains(nameLower, "_"+strings.ToLower(string(ed))) {
				seen[ed] = struct{}{}
			}
		}
	}

	result := make([]constants.Edition, 0, len(seen))
	for ed := range seen {
		result = append(result, ed)
	}
	return result
}

// GroupFilesByType groups files by their FileType field (e.g. "esd", "cab", "psf").
// FileType is normalised (lowercase, no leading dot) by the files service.
func GroupFilesByType(files []models.File) map[string][]models.File {
	groups := make(map[string][]models.File)
	for _, f := range files {
		groups[f.FileType] = append(groups[f.FileType], f)
	}
	return groups
}

// TotalSize returns the sum of SizeBytes across all files.
// Use to estimate required disk space before calling DownloadFiles.
func TotalSize(files []models.File) int64 {
	var total int64
	for _, f := range files {
		total += f.SizeBytes
	}
	return total
}
