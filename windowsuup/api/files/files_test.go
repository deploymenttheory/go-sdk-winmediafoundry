package files

import (
	"testing"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/constants"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/shared/models"
	"github.com/stretchr/testify/assert"
)

func TestApplyFileFilters_NoFilters(t *testing.T) {
	files := []models.File{
		{Name: "Windows11.0-26100.1-amd64.esd"},
		{Name: "lp.cab"},
	}
	got := applyFileFilters(files, &fileConfig{})
	assert.Equal(t, files, got)
}

func TestApplyFileFilters_ExtensionFilter(t *testing.T) {
	files := []models.File{
		{Name: "update.esd"},
		{Name: "lang.cab"},
		{Name: "meta.psf"},
	}
	got := applyFileFilters(files, &fileConfig{extension: ".esd"})
	assert.Len(t, got, 1)
	assert.Equal(t, "update.esd", got[0].Name)
}

func TestApplyFileFilters_ExtensionFilter_NoDot(t *testing.T) {
	files := []models.File{
		{Name: "update.esd"},
		{Name: "lang.cab"},
	}
	// Extension without leading dot should also work.
	got := applyFileFilters(files, &fileConfig{extension: "cab"})
	assert.Len(t, got, 1)
	assert.Equal(t, "lang.cab", got[0].Name)
}

func TestApplyFileFilters_LanguageFilter(t *testing.T) {
	files := []models.File{
		{Name: "Windows11.0-26100.1_en-us_Professional.esd"},
		{Name: "Windows11.0-26100.1_de-de_Professional.esd"},
		{Name: "Windows11.0-26100.1_neutral_Metadata.cab"},
		{Name: "Windows11.0-26100.1.cab"}, // no language marker → neutral
	}
	got := applyFileFilters(files, &fileConfig{language: "en-us"})
	names := make([]string, len(got))
	for i, f := range got {
		names[i] = f.Name
	}
	assert.Contains(t, names, "Windows11.0-26100.1_en-us_Professional.esd", "en-us file should be included")
	assert.Contains(t, names, "Windows11.0-26100.1_neutral_Metadata.cab", "neutral file should be included")
	assert.Contains(t, names, "Windows11.0-26100.1.cab", "no-marker file should be included")
	assert.NotContains(t, names, "Windows11.0-26100.1_de-de_Professional.esd", "de-de file should be excluded")
}

func TestApplyFileFilters_EditionFilter(t *testing.T) {
	files := []models.File{
		{Name: "file_professional_x64.esd"},
		{Name: "file_core_x64.esd"},
		{Name: "file_enterprise_x64.esd"},
		{Name: "metadata.cab"}, // no edition marker → always included
	}
	got := applyFileFilters(files, &fileConfig{edition: constants.EditionProfessional})
	names := make([]string, len(got))
	for i, f := range got {
		names[i] = f.Name
	}
	assert.Contains(t, names, "file_professional_x64.esd")
	assert.Contains(t, names, "metadata.cab", "no-edition-marker file should always pass")
	assert.NotContains(t, names, "file_core_x64.esd")
	assert.NotContains(t, names, "file_enterprise_x64.esd")
}

func TestApplyFileFilters_CombinedFilters(t *testing.T) {
	files := []models.File{
		{Name: "Windows11.0-26100.1_en-us_professional.esd"},
		{Name: "Windows11.0-26100.1_de-de_professional.esd"},
		{Name: "Windows11.0-26100.1_en-us_professional.cab"},
		{Name: "Windows11.0-26100.1_en-us_core.esd"},
	}
	got := applyFileFilters(files, &fileConfig{
		language:  "en-us",
		edition:   constants.EditionProfessional,
		extension: ".esd",
	})
	assert.Len(t, got, 1)
	assert.Equal(t, "Windows11.0-26100.1_en-us_professional.esd", got[0].Name)
}
