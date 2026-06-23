package cab

import (
	"errors"
	"testing"
)

func TestExtractNotCabinet(t *testing.T) {
	if _, err := Extract([]byte("not a cab at all, padding padding")); !errors.Is(err, errNotCabinet) {
		t.Fatalf("got %v, want errNotCabinet", err)
	}
	if _, err := Extract([]byte("short")); !errors.Is(err, errNotCabinet) {
		t.Fatalf("got %v, want errNotCabinet for short input", err)
	}
}

func TestExtractTruncatedHeaders(t *testing.T) {
	// Valid 36-byte CFHEADER claiming 1 folder and 1 file, but no folder/file
	// data follows, so folder parsing must fail as truncated.
	hdr := make([]byte, 36)
	copy(hdr, "MSCF")
	hdr[26], hdr[27] = 1, 0 // cFolders = 1
	hdr[28], hdr[29] = 1, 0 // cFiles = 1
	if _, err := Extract(hdr); !errors.Is(err, errTruncated) {
		t.Fatalf("got %v, want errTruncated", err)
	}
}

func TestExtractFileFromRealCab(t *testing.T) {
	data := readFixtureCab(t)

	if _, err := ExtractFile(data, "products.xml"); err != nil {
		t.Fatalf("ExtractFile(products.xml): %v", err)
	}
	if _, err := ExtractFile(data, "does-not-exist.bin"); !errors.Is(err, errFileNotFound) {
		t.Fatalf("got %v, want errFileNotFound", err)
	}
	if _, err := ExtractFile([]byte("bad"), "x"); err == nil {
		t.Fatal("expected error extracting from invalid cab")
	}
}

func TestReadCString(t *testing.T) {
	data := []byte("hello\x00world\x00")
	s, off, err := readCString(data, 0)
	if err != nil || s != "hello" || off != 6 {
		t.Fatalf("readCString = %q, %d, %v", s, off, err)
	}
	if next, err := skipCString(data, 6); err != nil || next != 12 {
		t.Fatalf("skipCString = %d, %v", next, err)
	}
	if _, _, err := readCString([]byte("unterminated"), 0); !errors.Is(err, errTruncated) {
		t.Fatalf("got %v, want errTruncated for unterminated string", err)
	}
}
