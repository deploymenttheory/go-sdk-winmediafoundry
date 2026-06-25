package wim

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

// TestWriteCompressedRoundTrip writes an XPRESS-compressed WIM and reads it back
// through this package's reader, verifying every file's bytes survive and that
// the data was genuinely compressed (multi-chunk + incompressible files too).
func TestWriteCompressedRoundTrip(t *testing.T) {
	dir := t.TempDir()
	// Highly compressible, spanning many 32 KiB chunks (forces the chunk table).
	big := bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. "), 200000) // ~9 MB
	// Incompressible random file (must be stored raw per-chunk yet round-trip).
	rnd := make([]byte, 300000)
	if _, err := rand.Read(rnd); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"big.txt":        big,
		"small.txt":      []byte("hello world"),
		"sub/random.bin": rnd,
		"sub/empty.txt":  {},
	}
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	out := filepath.Join(t.TempDir(), "test.wim")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewWriterCompressed(f, CompressionXPRESS)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.AddImage(dir, "Test"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	st, _ := os.Stat(out)
	rawTotal := len(big) + len(rnd) + 11
	t.Logf("compressed WIM = %d bytes (raw content ~%d, ratio %.1f%%)", st.Size(), rawTotal, 100*float64(st.Size())/float64(rawTotal))
	if st.Size() >= int64(rawTotal) {
		t.Errorf("WIM not smaller than raw content (%d >= %d) — compression not applied", st.Size(), rawTotal)
	}

	ww, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer ww.Close()
	if ww.Info().Compression != CompressionXPRESS {
		t.Fatalf("Info().Compression = %v, want XPRESS", ww.Info().Compression)
	}

	root, err := ww.OpenImage(1)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]byte{}
	var walk func(f *File, prefix string)
	walk = func(parent *File, prefix string) {
		for _, c := range parent.Children() {
			name := c.Name
			if prefix != "" {
				name = prefix + "/" + name
			}
			if c.IsDir() {
				walk(c, name)
				continue
			}
			data, rerr := ww.ReadFile(c)
			if rerr != nil {
				t.Fatalf("read %s: %v", name, rerr)
			}
			got[name] = data
		}
	}
	walk(root, "")

	for rel, want := range files {
		if len(want) == 0 {
			continue // empty files carry no blob
		}
		g, ok := got[rel]
		if !ok {
			t.Errorf("missing file %s", rel)
			continue
		}
		if !bytes.Equal(g, want) {
			t.Errorf("file %s: round-trip mismatch (got %d bytes, want %d)", rel, len(g), len(want))
		}
	}
}

// TestAddImageFromWIMCompressed exercises the exact path buildWIM uses for
// boot.wim/install.wim: copy an image from a source WIM into a compressed WIM.
func TestAddImageFromWIMCompressed(t *testing.T) {
	dir := t.TempDir()
	big := bytes.Repeat([]byte("compress me please, over and over. "), 200000) // ~7 MB
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), big, 0o644); err != nil {
		t.Fatal(err)
	}

	srcPath := filepath.Join(t.TempDir(), "src.wim")
	sf, err := os.Create(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := CreateFromDir(sf, dir, "Src"); err != nil {
		t.Fatal(err)
	}
	sf.Close()
	src, err := Open(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	outPath := filepath.Join(t.TempDir(), "out.wim")
	of, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewWriterCompressed(of, CompressionXPRESS)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.AddImageFromWIM(src, 1, "Out"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	of.Close()

	st, _ := os.Stat(outPath)
	t.Logf("AddImageFromWIM compressed: %d bytes (raw ~%d)", st.Size(), len(big))
	if st.Size() >= int64(len(big)) {
		t.Errorf("AddImageFromWIM did not compress (%d >= %d)", st.Size(), len(big))
	}

	out, err := Open(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if out.Info().Compression != CompressionXPRESS {
		t.Fatalf("Info().Compression = %v, want XPRESS", out.Info().Compression)
	}
	root, err := out.OpenImage(1)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range root.Children() {
		if c.IsDir() || c.Name != "big.txt" {
			continue
		}
		data, rerr := out.ReadFile(c)
		if rerr != nil {
			t.Fatalf("read big.txt: %v", rerr)
		}
		if !bytes.Equal(data, big) {
			t.Errorf("big.txt round-trip mismatch via AddImageFromWIM")
		}
	}
}
