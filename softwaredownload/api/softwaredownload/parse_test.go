package softwaredownload

import (
	"testing"
	"time"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/constants"
)

func TestParseEditions(t *testing.T) {
	html := []byte(`
		<select id="product-edition">
			<option value="">Select Download</option>
			<option value="3324">Windows 11 (multi-edition ISO for Arm64) </option>
		</select>`)

	got := parseEditions(html, Windows11ARM64, "https://example/arm")
	if len(got) != 1 {
		t.Fatalf("parseEditions returned %d products, want 1: %+v", len(got), got)
	}
	p := got[0]
	if p.EditionID != "3324" {
		t.Errorf("EditionID = %q, want 3324", p.EditionID)
	}
	if p.Name != "Windows 11 (multi-edition ISO for Arm64)" {
		t.Errorf("Name = %q (should be trimmed)", p.Name)
	}
	if p.Arch != constants.ArchARM64 {
		t.Errorf("Arch = %q, want ARM64", p.Arch)
	}
	if p.PageURL != "https://example/arm" {
		t.Errorf("PageURL = %q", p.PageURL)
	}
}

func TestParseEditionsArchFromName(t *testing.T) {
	// A page with no fixed Arch should infer it from the option label.
	html := []byte(`<option value="3321">Windows 11 (multi-edition ISO for x64 devices)</option>`)
	got := parseEditions(html, Page{Name: "generic", Path: "p"}, "u")
	if len(got) != 1 || got[0].Arch != constants.ArchX64 {
		t.Fatalf("expected x64 inferred from name, got %+v", got)
	}
}

func TestParseMDT(t *testing.T) {
	body := []byte(`window.mdt="https://ov-df.microsoft.com/?w=8DED15E8D920EF8&PageId=si";var rticks="+1782243316547;`)
	w, rticks := parseMDT(body)
	if w != "8DED15E8D920EF8" {
		t.Errorf("w = %q, want 8DED15E8D920EF8", w)
	}
	if rticks != "1782243316547" {
		t.Errorf("rticks = %q, want 1782243316547", rticks)
	}
}

func TestSelectSKU(t *testing.T) {
	info := skuInfoResponse{Skus: []skuEntry{
		{ID: "20086", Language: "English", LocalizedLanguage: "English (United States)"},
		{ID: "20087", Language: "English International", LocalizedLanguage: "English International"},
		{ID: "20075", Language: "Arabic", LocalizedLanguage: "Arabic"},
	}}

	cases := map[string]string{
		"English (United States)": "20086", // exact localized match (default)
		"english":                 "20086", // exact case-insensitive Language match
		"English International":   "20087",
		"Arabic":                  "20075",
	}
	for want, id := range cases {
		sku, ok := selectSKU(info, want)
		if !ok || sku.ID != id {
			t.Errorf("selectSKU(%q) = %q,%v; want %q", want, sku.ID, ok, id)
		}
	}

	if _, ok := selectSKU(info, "Klingon"); ok {
		t.Errorf("selectSKU(Klingon) should not match")
	}
}

func TestFileNameFromURL(t *testing.T) {
	url := "https://software.download.prss.microsoft.com/dbazure/Win11_25H2_English_Arm64_v2.iso?t=abc&P1=1782329719"
	if got := fileNameFromURL(url); got != "Win11_25H2_English_Arm64_v2.iso" {
		t.Errorf("fileNameFromURL = %q", got)
	}
}

func TestExpiryFromURL(t *testing.T) {
	url := "https://x/Win11.iso?t=abc&P1=1782329719&P2=602"
	got := expiryFromURL(url)
	if !got.Equal(time.Unix(1782329719, 0)) {
		t.Errorf("expiryFromURL = %v, want %v", got, time.Unix(1782329719, 0))
	}
	if !expiryFromURL("https://x/Win11.iso").IsZero() {
		t.Errorf("expiryFromURL without P1 should be zero")
	}
}

func TestPageURL(t *testing.T) {
	if got := Windows11ARM64.url("en-US"); got != "https://www.microsoft.com/en-US/software-download/windows11arm64" {
		t.Errorf("Page.url = %q", got)
	}
}
