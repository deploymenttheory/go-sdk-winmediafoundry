package soap

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── parseFileURLs ───────────────────────────────────────────────────────────

func TestParseFileURLs_FromFile(t *testing.T) {
	raw, err := os.ReadFile("../../../testdata/soap/get_extended_info_response.xml")
	require.NoError(t, err)

	urls, err := parseFileURLs(raw)
	require.NoError(t, err)

	// PSF file must be excluded; 2 valid files remain.
	require.Len(t, urls, 2, "PSF must be excluded; .esd and .cab should remain")

	byName := make(map[string]bool, len(urls))
	for _, u := range urls {
		byName[u.Name] = true
		assert.NotEmpty(t, u.URL)
		assert.NotEmpty(t, u.SHA1)
		// ExpiresAt from P1=1745798400
		assert.Equal(t, time.Unix(1745798400, 0).UTC(), u.ExpiresAt)
	}

	assert.True(t, byName["Windows11.0-26120.4061-amd64.esd"], "ESD file should be present")
	assert.True(t, byName["Windows11.0-26120.4061-lang.cab"], "CAB file should be present")
}

func TestParseFileURLs_Empty(t *testing.T) {
	raw := []byte(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetExtendedUpdateInfo2Response xmlns="http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService">
      <GetExtendedUpdateInfo2Result>
        <FileLocations/>
      </GetExtendedUpdateInfo2Result>
    </GetExtendedUpdateInfo2Response>
  </s:Body>
</s:Envelope>`)

	urls, err := parseFileURLs(raw)
	require.NoError(t, err)
	assert.Empty(t, urls)
}

func TestParseFileURLs_MalformedXML(t *testing.T) {
	_, err := parseFileURLs([]byte("not xml"))
	assert.Error(t, err)
}

// ─── shouldExclude ───────────────────────────────────────────────────────────

func TestShouldExclude(t *testing.T) {
	excluded := []string{
		"Windows11.0-KB12345-x64-EXPRESS.cab",
		"Windows10.0-KB12345-x64-EXPRESS.cab",
		"SSU-26100.1-amd64-EXPRESS.cab",
		"foo_Diffs_bar.cab",
		"foo_Forward_CompDB_bar.cab",
		"something.cbsu.cab",
		"baseless_something.cab",
		"patch.psf",
		"update.PSF", // case-insensitive
	}
	for _, name := range excluded {
		t.Run(name, func(t *testing.T) {
			assert.True(t, shouldExclude(name), "expected %q to be excluded", name)
		})
	}

	kept := []string{
		"Windows11.0-26120.4061-amd64.esd",
		"Windows11.0-26120.4061-amd64.cab",
		"lang-pack-en-us.cab",
		"metadata.cab",
	}
	for _, name := range kept {
		t.Run(name, func(t *testing.T) {
			assert.False(t, shouldExclude(name), "expected %q to NOT be excluded", name)
		})
	}
}

// ─── extractFilenameFromURL ───────────────────────────────────────────────────

func TestExtractFilenameFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://tlu.dl.delivery.mp.microsoft.com/filestreamingservice/files/abc/foo.esd?P1=123", "foo.esd"},
		{"https://example.com/a/b/c.cab", "c.cab"},
		{"https://example.com/file.cab?foo=bar&baz=qux", "file.cab"},
		{"not a url ://", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			assert.Equal(t, tt.want, extractFilenameFromURL(tt.url))
		})
	}
}

// ─── extractExpiry ───────────────────────────────────────────────────────────

func TestExtractExpiry(t *testing.T) {
	url := "https://tlu.dl.delivery.mp.microsoft.com/files/abc.esd?P1=1745798400&P2=x"
	got := extractExpiry(url)
	assert.Equal(t, time.Unix(1745798400, 0).UTC(), got)

	assert.True(t, extractExpiry("https://example.com/file").IsZero(), "no P1 → zero time")
	assert.True(t, extractExpiry("").IsZero(), "empty URL → zero time")
}
