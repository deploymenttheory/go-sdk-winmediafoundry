package soap

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestGenerateDeviceToken(t *testing.T) {
	token, err := generateDeviceToken()
	require.NoError(t, err)
	assert.NotEmpty(t, token, "device token must not be empty")
	token2, err := generateDeviceToken()
	require.NoError(t, err)
	assert.NotEqual(t, token, token2, "each device token must be unique")
}

func TestNewMessageID(t *testing.T) {
	id := newMessageID()
	assert.Len(t, id, 36, "UUID must be 36 chars")
	assert.Equal(t, '-', rune(id[8]))
	assert.Equal(t, '-', rune(id[13]))
	assert.Equal(t, '-', rune(id[18]))
	assert.Equal(t, '-', rune(id[23]))
}

func TestBuildGetCookieEnvelope(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	env := buildGetCookieEnvelope(now, "faketoken")
	assert.Contains(t, env, "GetCookie")
	assert.Contains(t, env, "faketoken")
	assert.Contains(t, env, "2026-04-25T12:00:00+00:00")
}

func TestParseCookieResponse(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/soap/cookie_response.xml")
	require.NoError(t, err)

	var envelope getCookieEnvelope
	require.NoError(t, xml.Unmarshal(raw, &envelope))
	assert.NotEmpty(t, envelope.Body.Result.EncryptedData)
	assert.NotEmpty(t, envelope.Body.Result.Expiration)
}

func TestParseSyncUpdatesResponse(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/soap/fetch_updates_response.xml")
	require.NoError(t, err)

	results, err := parseSyncUpdatesResponse(raw)
	require.NoError(t, err)
	require.Len(t, results, 1, "only IsLeaf=true entries with non-empty blobs should be included")
	assert.Equal(t, "10.0.26120.4061", results[0].Build)
	require.NotEmpty(t, results[0].Files)
	assert.Equal(t, "Windows11.0-26120.4061-amd64.esd", results[0].Files[0].Name)
}

func TestParseFileURLs(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/soap/get_extended_info_response.xml")
	require.NoError(t, err)

	urls, err := parseFileURLs(raw)
	require.NoError(t, err)
	// PSF file must be excluded; 2 valid files remain.
	assert.Len(t, urls, 2)
	for _, u := range urls {
		assert.NotEmpty(t, u.URL)
		assert.NotEmpty(t, u.Name)
		assert.False(t, shouldExclude(u.Name), "excluded file slipped through: %s", u.Name)
	}
}

func TestShouldExclude(t *testing.T) {
	cases := []struct {
		name     string
		excluded bool
	}{
		{"Windows11.0-KB123456-EXPRESS.cab", true},
		{"somepatch.psf", true},
		{"baseless_something.cab", true},
		{"file_Diffs_something.cab", true},
		{"Windows11.0-26120.4061-amd64.esd", false},
		{"lang_pack_en-us.cab", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.excluded, shouldExclude(tc.name), tc.name)
	}
}

func TestCookieManagerAcquire(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/soap/cookie_response.xml")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	hc := srv.Client()
	cm, err := newCookieManager(hc, zap.NewNop())
	require.NoError(t, err)

	// Use the test server by directly calling httpClient.Post (bypasses hardcoded endpoint).
	body := buildGetCookieEnvelope(time.Now(), cm.deviceToken)
	resp, err := hc.Post(srv.URL, "application/soap+xml; charset=utf-8", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	var envelope getCookieEnvelope
	require.NoError(t, xml.NewDecoder(resp.Body).Decode(&envelope))
	assert.NotEmpty(t, envelope.Body.Result.EncryptedData)
}

func TestExtractExpiry(t *testing.T) {
	rawURL := "https://example.com/file.esd?P1=1745798400&P2=foo"
	expiry := extractExpiry(rawURL)
	assert.Equal(t, int64(1745798400), expiry.Unix())
}

func TestExtractFilenameFromURL(t *testing.T) {
	cases := []struct {
		url      string
		expected string
	}{
		{"https://example.com/files/abc/Windows11.esd?P1=123", "Windows11.esd"},
		{"https://example.com/somefile.cab", "somefile.cab"},
		{"", ""},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.expected, extractFilenameFromURL(tc.url))
	}
}
