package softwaredownload

import (
	"html"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/constants"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/shared/models"
)

// editionOptionRE matches a product-edition <option value="NNNN">Label</option>
// in the software-download page. Only numeric values are products; the page's
// placeholder option carries an empty value and is skipped.
var editionOptionRE = regexp.MustCompile(`(?is)<option[^>]*\bvalue="(\d+)"[^>]*>(.*?)</option>`)

// parseEditions extracts the product editions advertised on a download page.
func parseEditions(data []byte, page Page, pageURL string) []models.Product {
	var out []models.Product
	for _, m := range editionOptionRE.FindAllSubmatch(data, -1) {
		id := string(m[1])
		name := strings.TrimSpace(html.UnescapeString(stripTags(string(m[2]))))
		if id == "" || name == "" {
			continue
		}
		arch := page.Arch
		if arch == "" {
			arch = constants.ArchFromToken(name)
		}
		out = append(out, models.Product{
			EditionID: id,
			Name:      name,
			Arch:      arch,
			PageURL:   pageURL,
		})
	}
	return out
}

var tagRE = regexp.MustCompile(`<[^>]+>`)

// stripTags removes any nested HTML tags from an option label.
func stripTags(s string) string { return tagRE.ReplaceAllString(s, "") }

// mdtWRE and mdtRticksRE extract the throwaway tokens ov-df hands back in its
// mdt.js response; they must be echoed in the follow-up ov-df request before the
// session is considered "human".
var (
	mdtWRE      = regexp.MustCompile(`[?&]w=([A-Fa-f0-9]+)`)
	mdtRticksRE = regexp.MustCompile(`rticks="?\+?(\d+)`)
)

// parseMDT extracts the w and rticks tokens from an ov-df mdt.js body.
func parseMDT(body []byte) (w, rticks string) {
	if m := mdtWRE.FindSubmatch(body); m != nil {
		w = string(m[1])
	}
	if m := mdtRticksRE.FindSubmatch(body); m != nil {
		rticks = string(m[1])
	}
	return w, rticks
}

// connectorError is a single error entry returned by the download-connector API.
type connectorError struct {
	Type  int    `json:"Type"`
	Value string `json:"Value"`
}

// skuEntry is a single SKU (one language) of a product edition.
type skuEntry struct {
	ID                string   `json:"Id"`
	Language          string   `json:"Language"`
	LocalizedLanguage string   `json:"LocalizedLanguage"`
	FriendlyFileNames []string `json:"FriendlyFileNames"`
}

// skuInfoResponse is the getskuinformationbyproductedition response.
type skuInfoResponse struct {
	Skus   []skuEntry       `json:"Skus"`
	Errors []connectorError `json:"Errors"`
}

// linksResponse is the GetProductDownloadLinksBySku response.
type linksResponse struct {
	ProductDownloadOptions []struct {
		Name         string `json:"Name"`
		Uri          string `json:"Uri"`
		DownloadType int    `json:"DownloadType"`
	} `json:"ProductDownloadOptions"`
	Errors []connectorError `json:"Errors"`
}

// firstError returns the first connector error message, or "".
func firstError(errs []connectorError) string {
	if len(errs) == 0 {
		return ""
	}
	return errs[0].Value
}

// fileNameFromURL returns the ISO file name from a signed download URL, e.g.
// ".../Win11_25H2_English_Arm64_v2.iso?t=..." -> "Win11_25H2_English_Arm64_v2.iso".
func fileNameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return path.Base(u.Path)
}

// expiryFromURL extracts the link expiry from the signed URL's P1 parameter
// (a Unix timestamp). Returns the zero time when absent or unparseable.
func expiryFromURL(rawURL string) time.Time {
	u, err := url.Parse(rawURL)
	if err != nil {
		return time.Time{}
	}
	p1 := u.Query().Get("P1")
	if p1 == "" {
		return time.Time{}
	}
	sec, err := strconv.ParseInt(p1, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}
