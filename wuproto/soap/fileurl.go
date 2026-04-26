package soap

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/deploymenttheory/go-sdk-windowsuup/wuproto"
)

// File exclusion patterns (from UUP dump PHP source, get.php).
var (
	reExcludeDiff     = regexp.MustCompile(`(?i).*_Diffs_.*|.*_Forward_CompDB_.*|\.cbsu\.cab$`)
	reExcludeBaseless = regexp.MustCompile(`(?i)^baseless_`)
	reExcludeEXPRESS  = regexp.MustCompile(`(?i)Windows(?:10|11)\.0-KB.*-EXPRESS|SSU-.*-EXPRESS`)
	reExcludePSF      = regexp.MustCompile(`(?i)\.psf$`)
)

// shouldExclude returns true if the filename should be excluded from results.
func shouldExclude(name string) bool {
	return reExcludeDiff.MatchString(name) ||
		reExcludeBaseless.MatchString(name) ||
		reExcludeEXPRESS.MatchString(name) ||
		reExcludePSF.MatchString(name)
}

// getFileURLs implements the GetExtendedUpdateInfo2 SOAP call.
func (c *SOAPClient) getFileURLs(ctx context.Context, req wuproto.FileURLRequest) ([]wuproto.FileURL, error) {
	// Device attributes for EUI2 must match the context of the originating
	// SyncUpdates call (arch, ring, build). Mismatched values cause the fe3cr
	// endpoint to return an empty GetExtendedUpdateInfo2Result.
	arch := string(req.Arch)
	if arch == "" {
		arch = "amd64"
	}
	ring := string(req.Ring)
	if ring == "" {
		ring = "Retail"
	}
	build := req.Build
	if build == "" {
		build = "10.0.26100.0"
	}
	deviceAttrs := buildDeviceAttributes(arch, ring, build, "", 48, "Production")

	body := buildGetEUI2Envelope(time.Now(), c.cookies.deviceToken, req.UpdateID, req.Revision, deviceAttrs)

	resp, err := c.cookies.post(ctx, clientSecuredEndpoint,
		"http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService/GetExtendedUpdateInfo2",
		body)
	if err != nil {
		return nil, fmt.Errorf("GetExtendedUpdateInfo2 SOAP call: %w", err)
	}
	defer resp.Body.Close()

	// Retry on cookie errors (same pattern as FetchUpdates).
	if resp.StatusCode == http.StatusInternalServerError {
		raw, _ := io.ReadAll(resp.Body)
		if isCookieError(string(raw)) {
			c.cookies.invalidate()
			_, err = c.cookies.get(ctx)
			if err != nil {
				return nil, fmt.Errorf("cookie refresh after EUI2 error: %w", err)
			}
			body = buildGetEUI2Envelope(time.Now(), c.cookies.deviceToken, req.UpdateID, req.Revision, deviceAttrs)
			resp, err = c.cookies.post(ctx, clientSecuredEndpoint,
				"http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService/GetExtendedUpdateInfo2",
				body)
			if err != nil {
				return nil, fmt.Errorf("GetExtendedUpdateInfo2 retry: %w", err)
			}
			defer resp.Body.Close()
		}
	}

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GetExtendedUpdateInfo2 returned HTTP %d: %s", resp.StatusCode, string(raw))
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read EUI2 response: %w", err)
	}

	return parseFileURLs(raw)
}

// parseFileURLs extracts FileURL values from a raw GetExtendedUpdateInfo2 XML response.
// Files are filtered (psf, diff, express) and deduplicated (keep largest size).
func parseFileURLs(raw []byte) ([]wuproto.FileURL, error) {
	var env getEUI2Envelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("unmarshal EUI2 response: %w", err)
	}

	// Deduplicate: sha1 → best FileURL (largest SizeBytes wins).
	byDigest := make(map[string]*wuproto.FileURL)

	for _, loc := range env.Body.Result.FileLocations {
		if loc.URL == "" {
			continue
		}

		name := extractFilenameFromURL(loc.URL)
		if name == "" || shouldExclude(name) {
			continue
		}

		sha1Hex := base64ToHex(loc.FileDigest)
		expiresAt := extractExpiry(loc.URL)

		candidate := &wuproto.FileURL{
			Name:      name,
			URL:       loc.URL,
			ExpiresAt: expiresAt,
			SHA1:      sha1Hex,
		}

		existing, ok := byDigest[sha1Hex]
		if !ok || (existing != nil && candidate.SizeBytes > existing.SizeBytes) {
			byDigest[sha1Hex] = candidate
		}
	}

	result := make([]wuproto.FileURL, 0, len(byDigest))
	for _, fu := range byDigest {
		if fu != nil {
			result = append(result, *fu)
		}
	}
	return result, nil
}

// extractFilenameFromURL attempts to extract the last path segment of a URL
// as the filename.
func extractFilenameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	p := u.Path
	if idx := strings.LastIndex(p, "/"); idx >= 0 {
		return p[idx+1:]
	}
	return p
}

// extractExpiry parses the P1 query parameter (a Unix timestamp) from a CDN URL.
//
// Example: https://...?P1=1713600000&...
func extractExpiry(rawURL string) time.Time {
	u, err := url.Parse(rawURL)
	if err != nil {
		return time.Time{}
	}
	p1 := u.Query().Get("P1")
	if p1 == "" {
		return time.Time{}
	}
	var ts int64
	if _, err := fmt.Sscanf(p1, "%d", &ts); err != nil {
		return time.Time{}
	}
	return time.Unix(ts, 0).UTC()
}
