package files

import (
	"testing"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/constants"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/shared/models"
	"github.com/stretchr/testify/assert"
)

func TestListLanguages(t *testing.T) {
	files := []models.File{
		{Name: "Windows11.0-26100.1_en-us_Professional.esd"},
		{Name: "Windows11.0-26100.1_de-de_Professional.esd"},
		{Name: "Windows11.0-26100.1_de-de_Core.esd"}, // duplicate lang — should not appear twice
		{Name: "Windows11.0-26100.1_neutral_Metadata.cab"},
		{Name: "Windows11.0-26100.1.cab"}, // no underscore → neutral, no language
		{Name: "lp-ja-jp.cab"},            // hyphen-dot pattern
	}

	langs := ListLanguages(files)

	assert.Contains(t, langs, "en-us")
	assert.Contains(t, langs, "de-de")
	assert.Contains(t, langs, "ja-jp")
	assert.NotContains(t, langs, "neutral", "neutral is not a BCP-47 language code")

	// Results must be sorted and deduplicated.
	assert.Equal(t, []string{"de-de", "en-us", "ja-jp"}, langs)
}

func TestListLanguages_Empty(t *testing.T) {
	assert.Empty(t, ListLanguages(nil))
	assert.Empty(t, ListLanguages([]models.File{{Name: "metadata.cab"}}))
}

func TestListEditions(t *testing.T) {
	files := []models.File{
		{Name: "file_professional_x64.esd"},
		{Name: "file_professional_x64.esd"}, // duplicate — deduplicated
		{Name: "file_core_x64.esd"},
		{Name: "file_enterprise_x64.esd"},
		{Name: "metadata.cab"}, // no edition marker — not included
	}

	eds := ListEditions(files)

	assert.Contains(t, eds, constants.EditionProfessional)
	assert.Contains(t, eds, constants.EditionHome) // "CORE" matches "_core"
	assert.Contains(t, eds, constants.EditionEnterprise)
	assert.NotContains(t, eds, constants.EditionEducation)

	// Deduplicated.
	seen := make(map[constants.Edition]int)
	for _, e := range eds {
		seen[e]++
	}
	for ed, count := range seen {
		assert.Equal(t, 1, count, "edition %q appeared more than once", ed)
	}
}

func TestListEditions_Empty(t *testing.T) {
	assert.Empty(t, ListEditions(nil))
	assert.Empty(t, ListEditions([]models.File{{Name: "metadata.cab"}}))
}

func TestGroupFilesByType(t *testing.T) {
	files := []models.File{
		{Name: "a.esd", FileType: "esd"},
		{Name: "b.esd", FileType: "esd"},
		{Name: "c.cab", FileType: "cab"},
		{Name: "d.psf", FileType: "psf"},
	}

	groups := GroupFilesByType(files)

	assert.Len(t, groups["esd"], 2)
	assert.Len(t, groups["cab"], 1)
	assert.Len(t, groups["psf"], 1)
	assert.Equal(t, "a.esd", groups["esd"][0].Name)
	assert.Equal(t, "b.esd", groups["esd"][1].Name)
}

func TestGroupFilesByType_Empty(t *testing.T) {
	assert.Empty(t, GroupFilesByType(nil))
	assert.Empty(t, GroupFilesByType([]models.File{}))
}

func TestTotalSize(t *testing.T) {
	files := []models.File{
		{SizeBytes: 1_000_000_000},  // 1 GB
		{SizeBytes: 500_000_000},    // 500 MB
		{SizeBytes: 250_000_000},    // 250 MB
	}
	assert.Equal(t, int64(1_750_000_000), TotalSize(files))
}

func TestTotalSize_Empty(t *testing.T) {
	assert.Equal(t, int64(0), TotalSize(nil))
	assert.Equal(t, int64(0), TotalSize([]models.File{}))
}

func TestTotalSize_ZeroSized(t *testing.T) {
	files := []models.File{{SizeBytes: 0}, {SizeBytes: 0}}
	assert.Equal(t, int64(0), TotalSize(files))
}
