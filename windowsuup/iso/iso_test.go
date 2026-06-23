package iso

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	diskfs "github.com/diskfs/go-diskfs"
)

// writeFile creates dir tree and a file with content under root.
func writeFile(t *testing.T, root, rel string, content []byte) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func buildMediaRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "boot/etfsboot.com", bytes.Repeat([]byte("B"), 2048))
	writeFile(t, root, "efi/microsoft/boot/efisys.bin", bytes.Repeat([]byte("E"), 4096))
	writeFile(t, root, "sources/boot.wim", []byte("fake boot.wim"))
	writeFile(t, root, "bootmgr", []byte("fake bootmgr"))
	writeFile(t, root, "setup.exe", []byte("fake setup"))
	writeFile(t, root, "autorun.inf", []byte("[autorun]\n"))
	return root
}

func TestBuildWindowsISO(t *testing.T) {
	root := buildMediaRoot(t)
	out := filepath.Join(t.TempDir(), "out.iso")

	if err := BuildWindowsISO(root, out, "TEST_WIN"); err != nil {
		t.Fatalf("BuildWindowsISO: %v", err)
	}

	// The image must contain the El Torito boot record volume descriptor.
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("EL TORITO SPECIFICATION")) {
		t.Error("ISO is missing the El Torito boot record descriptor")
	}

	// Reopen the ISO and verify a couple of files round-tripped.
	d, err := diskfs.Open(out, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("open iso: %v", err)
	}
	defer d.Close()
	fs, err := d.GetFilesystem(0)
	if err != nil {
		t.Fatalf("get filesystem: %v", err)
	}

	f, err := fs.OpenFile("/sources/boot.wim", os.O_RDONLY)
	if err != nil {
		t.Fatalf("open boot.wim in iso: %v", err)
	}
	got, _ := io.ReadAll(f)
	if string(got) != "fake boot.wim" {
		t.Errorf("boot.wim content = %q", got)
	}

	if _, err := fs.OpenFile("/bootmgr", os.O_RDONLY); err != nil {
		t.Errorf("bootmgr missing from iso: %v", err)
	}
}

func TestBuildPlainNoBoot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "dir/file.txt", []byte("hello"))
	out := filepath.Join(t.TempDir(), "plain.iso")

	// No boot entries: a data-only ISO, no El Torito.
	if err := Build(root, out, Options{VolumeID: "DATA"}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	d, err := diskfs.Open(out, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	fs, err := d.GetFilesystem(0)
	if err != nil {
		t.Fatal(err)
	}
	f, err := fs.OpenFile("/dir/file.txt", os.O_RDONLY)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	if got, _ := io.ReadAll(f); string(got) != "hello" {
		t.Errorf("content = %q", got)
	}
}

func TestBuildErrors(t *testing.T) {
	if err := Build(filepath.Join(t.TempDir(), "does-not-exist"), filepath.Join(t.TempDir(), "x.iso"), Options{}); err == nil {
		t.Error("expected error for missing source dir")
	}

	root := t.TempDir()
	writeFile(t, root, "a.txt", []byte("a"))
	if err := Build(root, filepath.Join(t.TempDir(), "no-such-dir", "x.iso"), Options{}); err == nil {
		t.Error("expected error creating image in a missing directory")
	}

	if err := BuildWindowsISO(t.TempDir(), filepath.Join(t.TempDir(), "x.iso"), "X"); err == nil {
		t.Error("expected error: no boot images in empty media root")
	}
}

func TestBuildUnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	root := t.TempDir()
	writeFile(t, root, "secret.bin", []byte("data"))
	p := filepath.Join(root, "secret.bin")
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(p, 0o644) //nolint:errcheck // restore so TempDir cleanup can remove it

	if err := Build(root, filepath.Join(t.TempDir(), "x.iso"), Options{}); err == nil {
		t.Error("expected error copying an unreadable file into the image")
	}
}

func TestWalkErrors(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	if err := os.Mkdir(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "locked/inner.txt", []byte("x"))
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o755) //nolint:errcheck // restore for cleanup

	// scanTree's walk error surfaces through Build.
	if err := Build(root, filepath.Join(t.TempDir(), "x.iso"), Options{}); err == nil {
		t.Error("expected scan error for unreadable directory")
	}
	// indexTree's walk error surfaces through WindowsBootEntries.
	if _, err := WindowsBootEntries(root); err == nil {
		t.Error("expected index error for unreadable directory")
	}
}

func TestWindowsBootEntries(t *testing.T) {
	root := buildMediaRoot(t)
	entries, err := WindowsBootEntries(root)
	if err != nil {
		t.Fatalf("WindowsBootEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Firmware != FirmwareBIOS || entries[0].LoadSize != biosBootLoadSize {
		t.Errorf("BIOS entry = %+v", entries[0])
	}
	if entries[1].Firmware != FirmwareUEFI {
		t.Errorf("UEFI entry = %+v", entries[1])
	}

	// Case-insensitive detection (Microsoft capitalises the EFI path).
	root2 := t.TempDir()
	writeFile(t, root2, "EFI/Microsoft/Boot/efisys.bin", []byte("x"))
	entries2, err := WindowsBootEntries(root2)
	if err != nil {
		t.Fatalf("case-insensitive: %v", err)
	}
	if len(entries2) != 1 || entries2[0].Firmware != FirmwareUEFI {
		t.Errorf("expected one UEFI entry, got %+v", entries2)
	}

	if _, err := WindowsBootEntries(t.TempDir()); err == nil {
		t.Error("expected error when no boot images present")
	}
}
