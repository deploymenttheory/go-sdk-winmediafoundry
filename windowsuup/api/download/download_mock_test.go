package download_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/api/download"
	downloadmocks "github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/api/download/mocks"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/shared/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnit_Download_DownloadFile_NoURL(t *testing.T) {
	svc := download.New(downloadmocks.NewDownloadSuccess(nil))

	resp, err := svc.DownloadFile(context.Background(), models.File{Name: "update.esd"}, t.TempDir())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no CDN URL")
	assert.Nil(t, resp)
}

func TestUnit_Download_DownloadFile_HappyPath(t *testing.T) {
	content := []byte("fake windows update payload")
	mock := downloadmocks.NewDownloadSuccess(content)
	svc := download.New(mock)

	destDir := t.TempDir()
	file := models.File{
		Name:      "file.esd",
		URL:       downloadmocks.TestCDNURL,
		SizeBytes: int64(len(content)),
	}

	resp, err := svc.DownloadFile(context.Background(), file, destDir)

	require.NoError(t, err)
	require.NotNil(t, resp)

	written, readErr := os.ReadFile(filepath.Join(destDir, "file.esd"))
	require.NoError(t, readErr)
	assert.Equal(t, content, written)
}

func TestUnit_Download_DownloadFile_SkipsExisting(t *testing.T) {
	content := []byte("already downloaded content")
	mock := downloadmocks.NewDownloadSuccess(content)
	svc := download.New(mock)

	destDir := t.TempDir()
	dest := filepath.Join(destDir, "file.esd")
	require.NoError(t, os.WriteFile(dest, content, 0o644))

	file := models.File{
		Name:      "file.esd",
		URL:       downloadmocks.TestCDNURL,
		SizeBytes: int64(len(content)),
	}

	resp, err := svc.DownloadFile(context.Background(), file, destDir)

	require.NoError(t, err)
	assert.Nil(t, resp) // nil resp means file was skipped
}

func TestUnit_Download_DownloadFiles_HappyPath(t *testing.T) {
	content := []byte("payload")
	mock := downloadmocks.NewDownloadSuccess(content)
	svc := download.New(mock)

	destDir := t.TempDir()
	files := []models.File{
		{Name: "file.esd", URL: downloadmocks.TestCDNURL, SizeBytes: int64(len(content))},
	}

	err := svc.DownloadFiles(context.Background(), files, destDir, 1)

	require.NoError(t, err)
	written, readErr := os.ReadFile(filepath.Join(destDir, "file.esd"))
	require.NoError(t, readErr)
	assert.Equal(t, content, written)
}

func TestUnit_Download_DownloadFiles_NoFiles(t *testing.T) {
	svc := download.New(downloadmocks.NewDownloadSuccess(nil))
	err := svc.DownloadFiles(context.Background(), nil, t.TempDir(), 0)
	assert.NoError(t, err)
}

func TestUnit_Download_DownloadFile_ExpiredURL(t *testing.T) {
	content := []byte("payload")
	mock := downloadmocks.NewDownloadSuccess(content)
	svc := download.New(mock)

	file := models.File{
		Name:      "file.esd",
		URL:       downloadmocks.TestCDNURL,
		SizeBytes: int64(len(content)),
		ExpiresAt: time.Now().Add(-1 * time.Minute), // already expired
	}

	resp, err := svc.DownloadFile(context.Background(), file, t.TempDir())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "CDN URL expired")
	assert.Nil(t, resp)
}

func TestUnit_Download_DownloadFile_WithProgress(t *testing.T) {
	content := []byte("fake windows update payload for progress test")
	mock := downloadmocks.NewDownloadSuccess(content)
	svc := download.New(mock)

	var buf bytes.Buffer
	destDir := t.TempDir()
	file := models.File{
		Name:      "progress.esd",
		URL:       downloadmocks.TestCDNURL,
		SizeBytes: int64(len(content)),
	}

	resp, err := svc.DownloadFile(context.Background(), file, destDir, download.WithProgress(&buf))

	require.NoError(t, err)
	require.NotNil(t, resp)

	// Progress bar should have written something to the buffer.
	assert.NotEmpty(t, buf.Bytes())

	written, readErr := os.ReadFile(filepath.Join(destDir, "progress.esd"))
	require.NoError(t, readErr)
	assert.Equal(t, content, written)
}

func TestUnit_Download_DownloadFile_WithProgressCallback(t *testing.T) {
	content := []byte("callback test payload")
	mock := downloadmocks.NewDownloadSuccess(content)
	svc := download.New(mock)

	var callbackFired bool
	cb := func(fileName string, written, total int64, elapsed time.Duration) {
		callbackFired = true
		assert.Equal(t, "cb.esd", fileName)
		assert.Greater(t, written, int64(0))
	}

	file := models.File{
		Name:      "cb.esd",
		URL:       downloadmocks.TestCDNURL,
		SizeBytes: int64(len(content)),
	}

	resp, err := svc.DownloadFile(context.Background(), file, t.TempDir(), download.WithProgressCallback(cb))

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, callbackFired)
}
