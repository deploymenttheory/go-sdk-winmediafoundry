package cab

import (
	"crypto/sha1" //nolint:gosec // fixture integrity check, not security
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExtractProductsCab decodes the real LZX-compressed Microsoft ESD catalog
// (products.cab) and checks the extracted products.xml byte-for-byte against the
// reference extraction produced by libarchive's bsdtar.
func TestExtractProductsCab(t *testing.T) {
	const (
		wantName = "products.xml"
		wantSize = 1743972
		wantSHA1 = "b0e49a9447c88e3aa6b4d188a77d93540699cf91"
	)

	data, err := os.ReadFile(filepath.Join("testdata", "products.cab"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	files, err := Extract(data)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	got := files[0]
	if got.Name != wantName {
		t.Errorf("name = %q, want %q", got.Name, wantName)
	}
	if len(got.Data) != wantSize {
		t.Fatalf("size = %d, want %d", len(got.Data), wantSize)
	}
	sum := sha1.Sum(got.Data) //nolint:gosec
	if h := hex.EncodeToString(sum[:]); h != wantSHA1 {
		t.Fatalf("sha1 = %s, want %s", h, wantSHA1)
	}
	if !strings.HasPrefix(string(got.Data), "<MCT>") {
		t.Errorf("decompressed content does not start with <MCT>")
	}
}
