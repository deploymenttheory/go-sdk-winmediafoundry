//go:build e2e

// End-to-end test of the full ESD pipeline against live Microsoft services:
// resolve the catalog, download a real install.esd, verify its SHA-1, open it
// as a WIM, enumerate images, and parse a Windows image's directory tree
// (decompressing LZMS metadata in pure Go).
//
// This downloads multiple gigabytes and takes many minutes, so it is gated
// behind the 'e2e' build tag and a long timeout:
//
//	go test -tags=e2e -timeout=60m -v ./windowsuup/acceptance/...
package acceptance_test

import (
	"context"
	"crypto/sha1" //nolint:gosec // catalog integrity check
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup"
	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/wim"
)

func newE2EClient(t *testing.T) *windowsuup.Client {
	t.Helper()
	c, err := windowsuup.NewClient()
	require.NoError(t, err)
	return c
}

func TestE2E_DownloadAndReadESD(t *testing.T) {
	c := newE2EClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	// 1. Resolve the catalog and pick the smallest real ESD to minimize bytes.
	cat, _, err := c.ESD.Catalog(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, cat.Images)
	imgs := cat.Images
	sort.Slice(imgs, func(i, j int) bool { return imgs[i].SizeBytes < imgs[j].SizeBytes })
	chosen := imgs[0]
	t.Logf("downloading %s (%.2f GB)", chosen.FileName, float64(chosen.SizeBytes)/1e9)

	// 2. Download it and verify the catalog SHA-1.
	dir := t.TempDir()
	_, err = c.Download.DownloadFile(ctx, chosen.AsFile(), dir)
	require.NoError(t, err)
	path := filepath.Join(dir, chosen.FileName)

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	h := sha1.New() //nolint:gosec
	_, err = io.Copy(h, f)
	require.NoError(t, err)
	assert.Equal(t, strings.ToLower(chosen.SHA1), hex.EncodeToString(h.Sum(nil)), "downloaded ESD SHA-1")

	// 3. Open it as a WIM and enumerate images.
	w, err := wim.Open(path)
	require.NoError(t, err)
	defer w.Close()
	assert.Equal(t, wim.CompressionLZMS, w.Info().Compression)
	require.NotEmpty(t, w.Images())

	// 4. Find a real Windows image and parse its directory tree.
	var winIndex int
	for _, im := range w.Images() {
		if strings.Contains(strings.ToLower(im.Name), "windows 11") {
			winIndex = im.Index
			break
		}
	}
	require.NotZero(t, winIndex, "expected a Windows 11 image")

	root, err := w.OpenImage(winIndex)
	require.NoError(t, err)
	require.True(t, root.IsDir())

	var files, dirs int
	var sample []*wim.File
	root.Walk(func(_ string, file *wim.File) {
		if file.IsDir() {
			dirs++
		} else {
			files++
			if len(sample) < 3 && !isZeroHash(file.Hash) {
				sample = append(sample, file)
			}
		}
	})
	t.Logf("image %d: %d directories, %d files", winIndex, dirs, files)
	assert.Greater(t, files, 1000, "a Windows image has many files")
	assert.Greater(t, dirs, 100)

	// 5. Extract a few real files out of the LZMS solid resource and verify each
	// against the SHA-1 the blob table is keyed by.
	require.NotEmpty(t, sample)
	for _, file := range sample {
		data, err := w.ReadFile(file)
		require.NoError(t, err, "ReadFile %s", file.Name)
		got := sha1.Sum(data) //nolint:gosec
		assert.Equal(t, file.Hash, got, "content SHA-1 of %s should match blob hash", file.Name)
	}
	require.NoError(t, ctx.Err())
}

func isZeroHash(h [20]byte) bool { return h == [20]byte{} }
