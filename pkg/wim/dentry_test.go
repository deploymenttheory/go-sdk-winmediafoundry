package wim

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseImageMetadata decompresses a real Windows 11 ESD image metadata
// resource (LZMS) and parses its directory-entry tree.
func TestParseImageMetadata(t *testing.T) {
	comp, err := os.ReadFile(filepath.Join("testdata", "lzms-metadata-resource.bin"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	cr, err := newChunkedReader(bytes.NewReader(comp), CompressionLZMS, 131072, int64(len(comp)), 206128)
	if err != nil {
		t.Fatalf("newChunkedReader: %v", err)
	}
	defer cr.Close()
	meta, err := io.ReadAll(cr)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}

	root, err := parseImageMetadata(meta)
	if err != nil {
		t.Fatalf("parseImageMetadata: %v", err)
	}
	if !root.IsDir() {
		t.Fatal("root is not a directory")
	}

	var files, dirs int
	names := map[string]bool{}
	root.Walk(func(path string, f *File) {
		if f.IsDir() {
			dirs++
		} else {
			files++
		}
		base := f.Name
		names[strings.ToLower(base)] = true
	})

	if files == 0 || dirs == 0 {
		t.Fatalf("expected files and dirs, got files=%d dirs=%d", files, dirs)
	}
	// A Windows install image's root contains a Windows directory (this small
	// "Windows Setup Media" image at least has recognizable setup files).
	t.Logf("parsed %d dirs, %d files", dirs, files)
	if len(names) < 5 {
		t.Errorf("suspiciously few distinct names: %d", len(names))
	}
}
