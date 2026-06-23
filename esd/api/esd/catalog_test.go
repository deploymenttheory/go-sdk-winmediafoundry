package esd

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	winmocks "github.com/deploymenttheory/winmediafoundry/esd/mocks"
)

func loadCab(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "products.cab"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

func TestCatalogSuccess(t *testing.T) {
	m := winmocks.NewGenericMock()
	m.Register("GET", Windows11.URL, http.StatusOK, loadCab(t))

	cat, resp, err := New(m).Catalog(context.Background())
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	if resp == nil {
		t.Error("expected a response")
	}
	if len(cat.Images) == 0 {
		t.Fatal("no images parsed")
	}
	// The catalog should expose multiple editions and languages.
	if len(cat.Editions()) < 2 || len(cat.Languages()) < 2 {
		t.Errorf("editions=%d languages=%d", len(cat.Editions()), len(cat.Languages()))
	}
	pro := cat.Filter("Professional", "x64", "en-us")
	if len(pro) == 0 {
		t.Error("expected an en-us x64 Professional image")
	}
}

func TestCatalogWithProduct(t *testing.T) {
	m := winmocks.NewGenericMock()
	m.Register("GET", Windows10.URL, http.StatusOK, loadCab(t))

	if _, _, err := New(m).Catalog(context.Background(), WithProduct(Windows10)); err != nil {
		t.Fatalf("Catalog(Windows10): %v", err)
	}
}

func TestCatalogHTTPError(t *testing.T) {
	m := winmocks.NewGenericMock()
	m.RegisterError("GET", Windows11.URL, http.StatusForbidden, "forbidden")

	if _, _, err := New(m).Catalog(context.Background()); err == nil {
		t.Fatal("expected error on HTTP failure")
	}
}

func TestCatalogBadCab(t *testing.T) {
	m := winmocks.NewGenericMock()
	m.Register("GET", Windows11.URL, http.StatusOK, []byte("definitely not a cabinet file"))

	if _, _, err := New(m).Catalog(context.Background()); err == nil {
		t.Fatal("expected error extracting products.xml from invalid cab")
	}
}

func TestParseBadXML(t *testing.T) {
	if _, err := parseCatalog([]byte("<MCT><not-closed")); err == nil {
		t.Fatal("expected XML parse error")
	}
}
