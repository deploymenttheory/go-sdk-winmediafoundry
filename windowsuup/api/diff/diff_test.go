package diff

import (
	"testing"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/shared/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffFileSets_Identical(t *testing.T) {
	files := []models.File{
		{Name: "a.esd", SHA256: "aaaa", SizeBytes: 100},
		{Name: "b.cab", SHA256: "bbbb", SizeBytes: 200},
	}
	buildA := models.Build{UUID: "uuid-a", Build: "10.0.26100.1"}
	buildB := models.Build{UUID: "uuid-b", Build: "10.0.26100.2"}

	d := diffFileSets(buildA, buildB, files, files)

	assert.Empty(t, d.Added)
	assert.Empty(t, d.Removed)
	assert.Empty(t, d.Changed)
	assert.Equal(t, 2, d.Unchanged)
	assert.Equal(t, "uuid-a", d.BaseUUID)
	assert.Equal(t, "uuid-b", d.TargetUUID)
}

func TestDiffFileSets_Added(t *testing.T) {
	base := []models.File{
		{Name: "a.esd", SHA256: "aaaa"},
	}
	target := []models.File{
		{Name: "a.esd", SHA256: "aaaa"},
		{Name: "b.cab", SHA256: "bbbb"},
	}

	d := diffFileSets(models.Build{}, models.Build{}, base, target)

	require.Len(t, d.Added, 1)
	assert.Equal(t, "b.cab", d.Added[0].Name)
	assert.Empty(t, d.Removed)
	assert.Empty(t, d.Changed)
	assert.Equal(t, 1, d.Unchanged)
}

func TestDiffFileSets_Removed(t *testing.T) {
	base := []models.File{
		{Name: "a.esd", SHA256: "aaaa"},
		{Name: "b.cab", SHA256: "bbbb"},
	}
	target := []models.File{
		{Name: "a.esd", SHA256: "aaaa"},
	}

	d := diffFileSets(models.Build{}, models.Build{}, base, target)

	assert.Empty(t, d.Added)
	require.Len(t, d.Removed, 1)
	assert.Equal(t, "b.cab", d.Removed[0].Name)
	assert.Empty(t, d.Changed)
	assert.Equal(t, 1, d.Unchanged)
}

func TestDiffFileSets_Changed(t *testing.T) {
	base := []models.File{
		{Name: "a.esd", SHA256: "aaaa", SizeBytes: 100},
	}
	target := []models.File{
		{Name: "a.esd", SHA256: "bbbb", SizeBytes: 200},
	}

	d := diffFileSets(models.Build{}, models.Build{}, base, target)

	assert.Empty(t, d.Added)
	assert.Empty(t, d.Removed)
	require.Len(t, d.Changed, 1)
	assert.Equal(t, "a.esd", d.Changed[0].Name)
	assert.Equal(t, int64(100), d.Changed[0].BaseFile.SizeBytes)
	assert.Equal(t, int64(200), d.Changed[0].TargetFile.SizeBytes)
	assert.Equal(t, 0, d.Unchanged)
}

func TestDiffFileSets_Empty(t *testing.T) {
	d := diffFileSets(models.Build{}, models.Build{}, nil, nil)
	assert.Empty(t, d.Added)
	assert.Empty(t, d.Removed)
	assert.Empty(t, d.Changed)
	assert.Equal(t, 0, d.Unchanged)
}

func TestFileChanged_SHA256(t *testing.T) {
	a := models.File{SHA256: "abc", SHA1: "111", SizeBytes: 100}
	b := models.File{SHA256: "abc", SHA1: "222", SizeBytes: 200}
	// SHA256 matches → not changed, even if SHA1 and size differ.
	assert.False(t, fileChanged(a, b))

	a.SHA256 = "abc"
	b.SHA256 = "xyz"
	assert.True(t, fileChanged(a, b))
}

func TestFileChanged_SHA1Fallback(t *testing.T) {
	a := models.File{SHA256: "", SHA1: "same"}
	b := models.File{SHA256: "", SHA1: "same"}
	assert.False(t, fileChanged(a, b))

	b.SHA1 = "different"
	assert.True(t, fileChanged(a, b))
}

func TestFileChanged_SizeFallback(t *testing.T) {
	a := models.File{SizeBytes: 1000}
	b := models.File{SizeBytes: 1000}
	assert.False(t, fileChanged(a, b))

	b.SizeBytes = 2000
	assert.True(t, fileChanged(a, b))
}
