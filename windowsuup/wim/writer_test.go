package wim

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func writeSrcFile(t *testing.T, root, rel string, content []byte) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestWriteReadRoundTrip writes a directory tree as an uncompressed WIM, reopens
// it with the reader, and extracts it back, verifying structure and content.
func TestWriteReadRoundTrip(t *testing.T) {
	src := t.TempDir()
	files := map[string][]byte{
		"readme.txt":      []byte("top-level file"),
		"dir/b.txt":       []byte("nested file b"),
		"dir/sub/c.bin":   bytes.Repeat([]byte{0xAB}, 5000),
		"dir/dup.txt":     []byte("nested file b"), // duplicate content -> dedup
		"empty.txt":       {},
		"unicode/日本.txt": []byte("utf-16 name"),
	}
	for rel, content := range files {
		writeSrcFile(t, src, rel, content)
	}
	if err := os.MkdirAll(filepath.Join(src, "emptydir"), 0o755); err != nil {
		t.Fatal(err)
	}

	wimPath := filepath.Join(t.TempDir(), "out.wim")
	out, err := os.Create(wimPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := CreateFromDir(out, src, "Test Image"); err != nil {
		out.Close()
		t.Fatalf("CreateFromDir: %v", err)
	}
	out.Close()

	w, err := Open(wimPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()

	if w.ImageCount() != 1 {
		t.Errorf("ImageCount = %d, want 1", w.ImageCount())
	}
	imgs := w.Images()
	if len(imgs) != 1 || imgs[0].Name != "Test Image" {
		t.Fatalf("images = %+v", imgs)
	}

	dest := t.TempDir()
	if err := w.ExtractImage(1, dest); err != nil {
		t.Fatalf("ExtractImage: %v", err)
	}

	// Every source file must reappear with identical content.
	for rel, want := range files {
		got, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("missing %s: %v", rel, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s: content mismatch (%d vs %d bytes)", rel, len(got), len(want))
		}
	}
	if st, err := os.Stat(filepath.Join(dest, "emptydir")); err != nil || !st.IsDir() {
		t.Errorf("emptydir not restored: %v", err)
	}
}

// TestWriterMultiImage builds a two-image WIM (the boot.wim shape) with content
// shared across images, and verifies both images extract correctly.
func TestWriterMultiImage(t *testing.T) {
	shared := bytes.Repeat([]byte("SHARED"), 500)

	src1 := t.TempDir()
	writeSrcFile(t, src1, "winpe.txt", []byte("PE image"))
	writeSrcFile(t, src1, "shared.bin", shared)
	src2 := t.TempDir()
	writeSrcFile(t, src2, "setup.txt", []byte("Setup image"))
	writeSrcFile(t, src2, "shared.bin", shared) // same content -> dedup across images

	wimPath := filepath.Join(t.TempDir(), "boot.wim")
	out, err := os.Create(wimPath)
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewWriter(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.AddImage(src1, "Windows PE"); err != nil {
		t.Fatal(err)
	}
	if err := w.AddImage(src2, "Windows Setup"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	out.Close()

	wim, err := Open(wimPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer wim.Close()
	if wim.ImageCount() != 2 {
		t.Fatalf("ImageCount = %d, want 2", wim.ImageCount())
	}
	names := []string{wim.Images()[0].Name, wim.Images()[1].Name}
	if names[0] != "Windows PE" || names[1] != "Windows Setup" {
		t.Errorf("image names = %v", names)
	}

	for idx, want := range map[int]string{1: "winpe.txt", 2: "setup.txt"} {
		dest := t.TempDir()
		if err := wim.ExtractImage(idx, dest); err != nil {
			t.Fatalf("ExtractImage(%d): %v", idx, err)
		}
		if _, err := os.Stat(filepath.Join(dest, want)); err != nil {
			t.Errorf("image %d missing %s: %v", idx, want, err)
		}
		got, err := os.ReadFile(filepath.Join(dest, "shared.bin"))
		if err != nil || !bytes.Equal(got, shared) {
			t.Errorf("image %d shared.bin mismatch: %v", idx, err)
		}
	}
}

func TestWriteSpecialCharsImageName(t *testing.T) {
	src := t.TempDir()
	writeSrcFile(t, src, "a.txt", []byte("x"))
	wimPath := filepath.Join(t.TempDir(), "out.wim")
	out, err := os.Create(wimPath)
	if err != nil {
		t.Fatal(err)
	}
	const name = `Win & Setup <Pro>`
	if err := CreateFromDir(out, src, name); err != nil {
		out.Close()
		t.Fatalf("CreateFromDir: %v", err)
	}
	out.Close()

	w, err := Open(wimPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()
	if got := w.Images()[0].Name; got != name {
		t.Errorf("image name = %q, want %q (XML escaping)", got, name)
	}
}

type failWriter struct{ budget int }

func (f *failWriter) Write(p []byte) (int, error) {
	if f.budget <= 0 {
		return 0, io.ErrShortWrite
	}
	n := min(len(p), f.budget)
	f.budget -= n
	if n < len(p) {
		return n, io.ErrShortWrite
	}
	return n, nil
}

func (f *failWriter) Seek(int64, int) (int64, error) { return 0, nil }

func TestCreateFromDirWriteError(t *testing.T) {
	src := t.TempDir()
	writeSrcFile(t, src, "a.txt", []byte("data"))
	if err := CreateFromDir(&failWriter{budget: 0}, src, "X"); err == nil {
		t.Error("expected write error")
	}
}

func TestCreateFromDirMissing(t *testing.T) {
	out, err := os.Create(filepath.Join(t.TempDir(), "x.wim"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if err := CreateFromDir(out, filepath.Join(t.TempDir(), "nope"), "X"); err == nil {
		t.Error("expected error for missing source dir")
	}
}
