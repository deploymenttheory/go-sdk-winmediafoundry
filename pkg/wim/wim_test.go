package wim

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestOpenRealESDHeader opens a fixture built from a real Windows 11 ESD's WIM
// header plus its uncompressed XML catalog, and checks the parsed header and
// image list.
func TestOpenRealESDHeader(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "sample-esd-header.bin"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	w, err := OpenReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("OpenReaderAt: %v", err)
	}
	defer w.Close()

	info := w.Info()
	if info.Compression != CompressionLZMS {
		t.Errorf("compression = %v, want LZMS", info.Compression)
	}
	if info.ChunkSize != 131072 {
		t.Errorf("chunkSize = %d, want 131072", info.ChunkSize)
	}
	if info.ImageCount != 5 {
		t.Errorf("imageCount = %d, want 5", info.ImageCount)
	}
	if !info.Solid {
		t.Errorf("expected solid resource flag")
	}

	images := w.Images()
	if len(images) != 5 {
		t.Fatalf("got %d images, want 5", len(images))
	}

	// Spot-check the two real Windows editions in this ESD.
	byName := map[string]ImageInfo{}
	for _, im := range images {
		byName[im.Name] = im
	}
	pro, ok := byName["Windows 11 Pro"]
	if !ok {
		t.Fatalf("missing 'Windows 11 Pro'; got %v", imageNames(images))
	}
	if pro.Edition != "Professional" {
		t.Errorf("Pro edition = %q, want Professional", pro.Edition)
	}
	if pro.Architecture != "arm64" {
		t.Errorf("Pro arch = %q, want arm64", pro.Architecture)
	}
	if ent, ok := byName["Windows 11 Enterprise"]; !ok || ent.Edition != "Enterprise" {
		t.Errorf("missing/incorrect Enterprise image: %+v", ent)
	}
}

func TestOpenInvalid(t *testing.T) {
	if _, err := OpenReaderAt(bytes.NewReader([]byte("not a wim, padding padding padding")), 34); err == nil {
		t.Fatal("expected error for non-WIM input")
	}
	if _, err := OpenReaderAt(bytes.NewReader([]byte("MSWIM")), 5); err == nil {
		t.Fatal("expected error for short header")
	}
}

func imageNames(images []ImageInfo) []string {
	var n []string
	for _, im := range images {
		n = append(n, im.Name)
	}
	return n
}
