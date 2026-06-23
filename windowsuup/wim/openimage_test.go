package wim

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// buildWIMWithMetadata constructs a WIM consisting of a header, a one-entry
// offset table, and a real LZMS-compressed metadata resource, so OpenImage
// exercises the full offset-table -> decompress -> dentry-tree path.
func buildWIMWithMetadata(t *testing.T) []byte {
	t.Helper()
	const (
		metaOrigSize = 206128
		metaSHA1     = "c6d0872da60175857d807d5714f49240b6adcff9"
	)
	meta, err := os.ReadFile(filepath.Join("testdata", "lzms-metadata-resource.bin"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	le := binary.LittleEndian
	const tableOffset = headerSize
	const tableSize = blobTableEntrySize
	metaOffset := tableOffset + tableSize

	hdr := make([]byte, headerSize)
	copy(hdr, imageTag[:])
	le.PutUint32(hdr[12:], 0x00000e00)
	le.PutUint32(hdr[16:], flagCompressLZMS|flagCompressed)
	le.PutUint32(hdr[20:], 131072)
	le.PutUint32(hdr[44:], 1) // image count
	// OffsetTable resource descriptor (offset 48): uncompressed.
	le.PutUint64(hdr[48:], uint64(tableSize)) // size, flags 0
	le.PutUint64(hdr[56:], tableOffset)
	le.PutUint64(hdr[64:], tableSize)

	// One offset-table entry describing the metadata resource.
	entry := make([]byte, blobTableEntrySize)
	le.PutUint64(entry[0:], uint64(resFlagMetadata|resFlagCompressed)<<56|uint64(len(meta)))
	le.PutUint64(entry[8:], uint64(metaOffset))
	le.PutUint64(entry[16:], metaOrigSize)
	le.PutUint16(entry[24:], 1) // part number
	le.PutUint32(entry[26:], 1) // refcount
	hash, _ := hex.DecodeString(metaSHA1)
	copy(entry[30:], hash)

	out := append(hdr, entry...)
	out = append(out, meta...)
	return out
}

func TestOpenImageFromOffsetTable(t *testing.T) {
	data := buildWIMWithMetadata(t)
	w, err := OpenReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("OpenReaderAt: %v", err)
	}
	if w.ImageCount() != 1 {
		t.Errorf("ImageCount = %d, want 1", w.ImageCount())
	}

	root, err := w.OpenImage(1)
	if err != nil {
		t.Fatalf("OpenImage: %v", err)
	}
	if !root.IsDir() {
		t.Fatal("root is not a directory")
	}

	var files, dirs int
	root.Walk(func(_ string, f *File) {
		if f.IsDir() {
			dirs++
		} else {
			files++
		}
	})
	if files < 100 || dirs < 10 {
		t.Fatalf("unexpected tree: files=%d dirs=%d", files, dirs)
	}

	// Out-of-range image indices.
	if _, err := w.OpenImage(0); err == nil {
		t.Error("expected error for image index 0")
	}
	if _, err := w.OpenImage(99); err == nil {
		t.Error("expected error for image index 99")
	}
}

func TestParseImageMetadataErrors(t *testing.T) {
	if _, err := parseImageMetadata([]byte{1, 2, 3}); err == nil {
		t.Error("expected error for too-short metadata")
	}
	// Valid 8-byte security header but a truncated/zero dentry tree.
	meta := make([]byte, 16) // security header (8) + 8 zero bytes (empty root list)
	if _, err := parseImageMetadata(meta); err == nil {
		t.Error("expected error for metadata with no root entry")
	}
}
