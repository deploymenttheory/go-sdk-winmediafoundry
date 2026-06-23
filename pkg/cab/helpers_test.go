package cab

import (
	"os"
	"path/filepath"
	"testing"
)

// readFixtureCab returns the bytes of the committed real products.cab fixture.
func readFixtureCab(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "products.cab"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}
