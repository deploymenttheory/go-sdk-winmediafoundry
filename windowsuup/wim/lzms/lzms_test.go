package lzms

import (
	"crypto/sha1" //nolint:gosec // integrity check against a known WIM blob hash
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// TestDecompressRealChunks decompresses the two LZMS chunks of a real Windows 11
// ESD metadata resource and verifies the concatenated output against the SHA-1
// recorded in the WIM blob table.
func TestDecompressRealChunks(t *testing.T) {
	const (
		chunk0Out = 131072
		chunk1Out = 75056 // 206128 - 131072
		wantSHA1  = "c6d0872da60175857d807d5714f49240b6adcff9"
	)

	data, err := os.ReadFile(filepath.Join("..", "testdata", "lzms-metadata-resource.bin"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	// Chunk table: one 4-byte entry giving chunk 1's offset past the table.
	ch1 := int(binary.LittleEndian.Uint32(data))
	chunk0 := data[4 : 4+ch1]
	chunk1 := data[4+ch1:]

	out0, err := Decompress(chunk0, chunk0Out)
	if err != nil {
		t.Fatalf("Decompress chunk 0: %v", err)
	}
	out1, err := Decompress(chunk1, chunk1Out)
	if err != nil {
		t.Fatalf("Decompress chunk 1: %v", err)
	}

	sum := sha1.Sum(append(out0, out1...)) //nolint:gosec
	if got := hex.EncodeToString(sum[:]); got != wantSHA1 {
		t.Fatalf("sha1 = %s, want %s", got, wantSHA1)
	}
}

func TestDecompressInvalid(t *testing.T) {
	if _, err := Decompress([]byte{1, 2, 3}, 10); err == nil {
		t.Error("expected error for odd-length input")
	}
	if _, err := Decompress([]byte{1, 2}, 10); err == nil {
		t.Error("expected error for too-short input")
	}
}
