package esd

import "testing"

func sampleCatalog() *ESDCatalog {
	return &ESDCatalog{Images: []ESDImage{
		{FileName: "a.esd", Edition: "Professional", Architecture: "x64", LanguageCode: "en-us", SizeBytes: 100, SHA1: "aa", URL: "http://x/a"},
		{FileName: "b.esd", Edition: "Enterprise", Architecture: "x64", LanguageCode: "en-us", SizeBytes: 200, SHA1: "bb", URL: "http://x/b"},
		{FileName: "c.esd", Edition: "Professional", Architecture: "ARM64", LanguageCode: "fr-fr", SizeBytes: 300, SHA1: "cc", URL: "http://x/c"},
	}}
}

func TestESDImageAsFile(t *testing.T) {
	img := sampleCatalog().Images[0]
	f := img.AsFile()
	if f.Name != "a.esd" || f.URL != "http://x/a" || f.SizeBytes != 100 || f.FileType != "esd" {
		t.Fatalf("File() = %+v", f)
	}
	if f.SHA1 != "" {
		t.Errorf("expected empty File.SHA1 (catalog SHA-1 is hex, not base64); got %q", f.SHA1)
	}
}

func TestESDCatalogFilter(t *testing.T) {
	c := sampleCatalog()

	if got := c.Filter("", "", ""); len(got) != 3 {
		t.Errorf("Filter(all) = %d, want 3", len(got))
	}
	if got := c.Filter("Professional", "", ""); len(got) != 2 {
		t.Errorf("Filter(Pro) = %d, want 2", len(got))
	}
	if got := c.Filter("professional", "x64", "EN-US"); len(got) != 1 || got[0].FileName != "a.esd" {
		t.Errorf("case-insensitive filter = %+v", got)
	}
	if got := c.Filter("Enterprise", "ARM64", ""); len(got) != 0 {
		t.Errorf("Filter(no match) = %d, want 0", len(got))
	}
}

func TestESDCatalogDistinct(t *testing.T) {
	c := sampleCatalog()
	if got := c.Editions(); len(got) != 2 {
		t.Errorf("Editions = %v, want 2 distinct", got)
	}
	if got := c.Architectures(); len(got) != 2 {
		t.Errorf("Architectures = %v, want 2 distinct", got)
	}
	if got := c.Languages(); len(got) != 2 {
		t.Errorf("Languages = %v, want 2 distinct", got)
	}
}
