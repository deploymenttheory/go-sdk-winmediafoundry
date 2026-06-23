package wim

import (
	"bytes"
	"crypto/sha1" //nolint:gosec // WIM blob integrity check, not security
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestLZMSMetadataResource decompresses a real, non-solid LZMS-compressed WIM
// metadata resource taken from a Windows 11 ESD and verifies it against the
// SHA-1 recorded in the WIM blob table.
func TestLZMSMetadataResource(t *testing.T) {
	const (
		originalSize = 206128
		chunkSize    = 131072
		wantSHA1     = "c6d0872da60175857d807d5714f49240b6adcff9"
	)

	comp, err := os.ReadFile(filepath.Join("testdata", "lzms-metadata-resource.bin"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	cr, err := newChunkedReader(bytes.NewReader(comp), CompressionLZMS,
		chunkSize, int64(len(comp)), originalSize)
	if err != nil {
		t.Fatalf("newChunkedReader: %v", err)
	}
	defer cr.Close()

	out, err := io.ReadAll(cr)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if len(out) != originalSize {
		t.Fatalf("decompressed size = %d, want %d", len(out), originalSize)
	}
	sum := sha1.Sum(out) //nolint:gosec
	if got := hex.EncodeToString(sum[:]); got != wantSHA1 {
		t.Fatalf("sha1 = %s, want %s", got, wantSHA1)
	}
}
