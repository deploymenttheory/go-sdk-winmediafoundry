package download

import (
	"crypto/sha1"  //nolint:gosec
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/shared/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hashSHA256 returns the base64-encoded SHA256 of data.
func hashSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return base64.StdEncoding.EncodeToString(h[:])
}

// hashSHA1 returns the base64-encoded SHA1 of data.
func hashSHA1(data []byte) string {
	h := sha1.Sum(data) //nolint:gosec
	return base64.StdEncoding.EncodeToString(h[:])
}

func TestVerifyFiles_Missing(t *testing.T) {
	dir := t.TempDir()
	files := []models.File{{Name: "missing.esd", SizeBytes: 100}}

	results, err := VerifyFiles(files, dir)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.False(t, results[0].OK)
	assert.Equal(t, "missing", results[0].Reason)
}

func TestVerifyFiles_SizeMismatch(t *testing.T) {
	dir := t.TempDir()
	data := []byte("hello world")
	name := "update.esd"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o644))

	files := []models.File{{Name: name, SizeBytes: 999}}
	results, err := VerifyFiles(files, dir)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.False(t, results[0].OK)
	assert.Contains(t, results[0].Reason, "size mismatch")
}

func TestVerifyFiles_SHA256Match(t *testing.T) {
	dir := t.TempDir()
	data := []byte("windows update payload")
	name := "update.esd"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o644))

	files := []models.File{
		{Name: name, SizeBytes: int64(len(data)), SHA256: hashSHA256(data)},
	}
	results, err := VerifyFiles(files, dir)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].OK)
	assert.Empty(t, results[0].Reason)
}

func TestVerifyFiles_SHA256Mismatch(t *testing.T) {
	dir := t.TempDir()
	data := []byte("correct content")
	wrongData := []byte("different content")
	name := "update.esd"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o644))

	files := []models.File{
		{Name: name, SizeBytes: int64(len(data)), SHA256: hashSHA256(wrongData)},
	}
	results, err := VerifyFiles(files, dir)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.False(t, results[0].OK)
	assert.Equal(t, "sha256 mismatch", results[0].Reason)
}

func TestVerifyFiles_SHA1FallbackMatch(t *testing.T) {
	dir := t.TempDir()
	data := []byte("windows update payload")
	name := "update.cab"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o644))

	// No SHA256 — falls back to SHA1.
	files := []models.File{
		{Name: name, SizeBytes: int64(len(data)), SHA1: hashSHA1(data)},
	}
	results, err := VerifyFiles(files, dir)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].OK)
}

func TestVerifyFiles_SHA1FallbackMismatch(t *testing.T) {
	dir := t.TempDir()
	data := []byte("correct content")
	wrongData := []byte("different content")
	name := "update.cab"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o644))

	files := []models.File{
		{Name: name, SizeBytes: int64(len(data)), SHA1: hashSHA1(wrongData)},
	}
	results, err := VerifyFiles(files, dir)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.False(t, results[0].OK)
	assert.Equal(t, "sha1 mismatch", results[0].Reason)
}

func TestVerifyFiles_SizeOnlyNoHashes(t *testing.T) {
	dir := t.TempDir()
	data := []byte("payload with no hash metadata")
	name := "update.psf"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o644))

	// No SHA256 or SHA1 — size match is sufficient.
	files := []models.File{
		{Name: name, SizeBytes: int64(len(data))},
	}
	results, err := VerifyFiles(files, dir)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].OK)
}

func TestVerifyFiles_MultipleFiles(t *testing.T) {
	dir := t.TempDir()

	goodData := []byte("good file")
	goodName := "good.esd"
	require.NoError(t, os.WriteFile(filepath.Join(dir, goodName), goodData, 0o644))

	files := []models.File{
		{Name: goodName, SizeBytes: int64(len(goodData)), SHA256: hashSHA256(goodData)},
		{Name: "missing.cab", SizeBytes: 1000},
	}

	results, err := VerifyFiles(files, dir)
	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.True(t, results[0].OK)
	assert.False(t, results[1].OK)
	assert.Equal(t, "missing", results[1].Reason)
}

func TestVerifyFiles_Empty(t *testing.T) {
	results, err := VerifyFiles(nil, t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, results)
}
