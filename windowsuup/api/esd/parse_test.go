package esd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deploymenttheory/go-sdk-windowsuup/pkg/cab"
)

// TestParseRealCatalog exercises the full path the Catalog method uses, minus
// the network: decompress the committed products.cab, extract products.xml, and
// parse it into ESD images.
func TestParseRealCatalog(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "products.cab"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	xmlData, err := cab.ExtractFile(data, productsXMLName)
	if err != nil {
		t.Fatalf("ExtractFile: %v", err)
	}

	cat, err := parseCatalog(xmlData)
	if err != nil {
		t.Fatalf("parseCatalog: %v", err)
	}
	if len(cat.Images) == 0 {
		t.Fatal("no images parsed")
	}

	// Every image must carry the fields needed to download and verify.
	for _, img := range cat.Images {
		if img.FileName == "" || img.URL == "" || img.SizeBytes == 0 ||
			img.Edition == "" || img.Architecture == "" || img.LanguageCode == "" {
			t.Fatalf("incomplete image: %+v", img)
			break
		}
		if !strings.HasSuffix(img.FileName, ".esd") {
			t.Errorf("unexpected file name %q", img.FileName)
		}
		if !strings.HasPrefix(img.URL, "http") {
			t.Errorf("unexpected URL %q", img.URL)
		}
	}

	// Spot-check filtering and AsFile adaptation.
	enUSx64 := cat.Filter("Professional", "x64", "en-us")
	if len(enUSx64) == 0 {
		t.Fatal("expected at least one en-us x64 Professional ESD")
	}
	f := enUSx64[0].AsFile()
	if f.FileType != "esd" || f.URL == "" || f.Name == "" {
		t.Errorf("AsFile produced incomplete File: %+v", f)
	}

	t.Logf("parsed %d ESD images; %d editions, %d languages",
		len(cat.Images), len(cat.Editions()), len(cat.Languages()))
}
