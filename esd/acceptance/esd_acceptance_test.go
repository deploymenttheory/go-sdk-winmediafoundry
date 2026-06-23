//go:build acceptance

// Acceptance tests for the ESD media catalog client. These make a live request
// to Microsoft's Media Creation Tool fwlink, decompress the returned
// products.cab (pure-Go LZX), and parse the ESD catalog.
//
// Run with:
//
//	go test -tags=acceptance -timeout=120s -v ./esd/...
package acceptance_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/deploymenttheory/winmediafoundry/esd"
	esdapi "github.com/deploymenttheory/winmediafoundry/esd/api/esd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAcceptance_ESDCatalog_Windows11(t *testing.T) {
	c, err := esd.NewClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cat, resp, err := c.Catalog(ctx, esdapi.WithProduct(esdapi.Windows11))
	require.NoError(t, err, "Catalog should fetch and parse products.cab")
	require.NotNil(t, resp)
	require.NotEmpty(t, cat.Images, "catalog should contain ESD images")

	// A real catalog exposes many editions, languages, and both architectures.
	assert.GreaterOrEqual(t, len(cat.Editions()), 5)
	assert.GreaterOrEqual(t, len(cat.Languages()), 5)
	assert.GreaterOrEqual(t, len(cat.Architectures()), 1)

	pro := cat.Filter("Professional", "x64", "en-us")
	require.NotEmpty(t, pro, "expected an en-us x64 Professional ESD")
	img := pro[0]
	assert.True(t, strings.HasPrefix(img.URL, "http"), "direct CDN URL")
	assert.True(t, strings.HasSuffix(img.FileName, ".esd"))
	assert.Greater(t, img.SizeBytes, int64(1<<30), "an install ESD is multiple GB")
	assert.Len(t, img.SHA1, 40, "hex SHA-1")

	f := img.AsFile()
	assert.Equal(t, "esd", f.FileType)
	assert.Equal(t, img.URL, f.URL)
}
