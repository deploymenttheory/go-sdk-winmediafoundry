package builder

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInjectExtraFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sources"), 0o755); err != nil {
		t.Fatal(err)
	}
	extras := map[string][]byte{
		"autounattend.xml":  []byte("<unattend/>"),
		"sources/extra.cmd": []byte("echo hi"),
		"../escape.txt":     []byte("anchored, not escaped"),
	}
	if err := injectExtraFiles(root, extras); err != nil {
		t.Fatalf("injectExtraFiles: %v", err)
	}

	if b, err := os.ReadFile(filepath.Join(root, "autounattend.xml")); err != nil || string(b) != "<unattend/>" {
		t.Fatalf("root autounattend.xml: got %q err=%v", b, err)
	}
	if b, err := os.ReadFile(filepath.Join(root, "sources", "extra.cmd")); err != nil || string(b) != "echo hi" {
		t.Fatalf("sources/extra.cmd: got %q err=%v", b, err)
	}
	// "../escape.txt" must be anchored INTO the media root, never written above it.
	if _, err := os.Stat(filepath.Join(root, "escape.txt")); err != nil {
		t.Fatalf("traversal path not anchored into media root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("file escaped the media root via traversal")
	}
}

func TestInjectExtraFilesNil(t *testing.T) {
	if err := injectExtraFiles(t.TempDir(), nil); err != nil {
		t.Fatalf("nil extras should be a no-op: %v", err)
	}
}
