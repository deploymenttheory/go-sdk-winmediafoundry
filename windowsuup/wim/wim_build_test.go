package wim

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"
)

// buildWIM assembles a minimal WIM: a 208-byte header with the given flags and
// chunk size, followed by the UTF-16LE-encoded XML catalog placed at offset 208.
func buildWIM(flags, chunkSize uint32, imageCount int, xmlText string) []byte {
	le := binary.LittleEndian

	// Encode XML as UTF-16LE with a BOM.
	u16 := utf16.Encode([]rune(xmlText))
	xmlLE := make([]byte, 2+len(u16)*2)
	xmlLE[0], xmlLE[1] = 0xFF, 0xFE
	for i, c := range u16 {
		le.PutUint16(xmlLE[2+i*2:], c)
	}

	hdr := make([]byte, headerSize)
	copy(hdr, imageTag[:])
	le.PutUint32(hdr[8:], headerSize)  // cbSize
	le.PutUint32(hdr[12:], 0x00010000) // version
	le.PutUint32(hdr[16:], flags)      // flags
	le.PutUint32(hdr[20:], chunkSize)  // chunk size
	le.PutUint32(hdr[44:], uint32(imageCount))
	// XMLData resource descriptor at offset 72: size+flags(8), offset(8), orig(8)
	le.PutUint64(hdr[72:], uint64(len(xmlLE))) // size, flags=0 (uncompressed)
	le.PutUint64(hdr[80:], headerSize)         // offset
	le.PutUint64(hdr[88:], uint64(len(xmlLE))) // original size

	return append(hdr, xmlLE...)
}

const sampleXML = `<WIM><IMAGE INDEX="1"><NAME>Img A</NAME><DESCRIPTION>first</DESCRIPTION>` +
	`<DIRCOUNT>10</DIRCOUNT><FILECOUNT>20</FILECOUNT><TOTALBYTES>12345</TOTALBYTES>` +
	`<WINDOWS><ARCH>9</ARCH><EDITIONID>Professional</EDITIONID>` +
	`<INSTALLATIONTYPE>Client</INSTALLATIONTYPE>` +
	`<LANGUAGES><LANGUAGE>en-US</LANGUAGE></LANGUAGES></WINDOWS></IMAGE>` +
	`<IMAGE INDEX="2"><NAME>Img B</NAME><WINDOWS><ARCH>12</ARCH></WINDOWS></IMAGE></WIM>`

func TestBuildAndOpenSyntheticWIM(t *testing.T) {
	cases := []struct {
		flags uint32
		want  Compression
	}{
		{flagCompressXP | flagCompressed, CompressionXPRESS},
		{flagCompressLZX | flagCompressed, CompressionLZX},
		{0, CompressionNone},
	}
	for _, c := range cases {
		data := buildWIM(c.flags, 32768, 2, sampleXML)
		w, err := OpenReaderAt(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatalf("flags=0x%x: OpenReaderAt: %v", c.flags, err)
		}
		if got := w.Info().Compression; got != c.want {
			t.Errorf("flags=0x%x: compression=%v, want %v", c.flags, got, c.want)
		}
		if got := w.Info().Compression.String(); got != c.want.String() || got == "" {
			t.Errorf("String() = %q", got)
		}
		images := w.Images()
		if len(images) != 2 {
			t.Fatalf("got %d images, want 2", len(images))
		}
		if images[0].Name != "Img A" || images[0].Edition != "Professional" || images[0].Architecture != "x64" {
			t.Errorf("image[0] = %+v", images[0])
		}
		if images[0].FileCount != 20 || images[0].TotalBytes != 12345 {
			t.Errorf("image[0] counts = %+v", images[0])
		}
		if images[1].Architecture != "arm64" {
			t.Errorf("image[1] arch = %q, want arm64", images[1].Architecture)
		}
		if w.XML() == "" {
			t.Error("XML() empty")
		}
	}
}

func TestOpenPath(t *testing.T) {
	data := buildWIM(flagCompressLZMS|flagCompressed, 131072, 1,
		`<WIM><IMAGE INDEX="1"><NAME>Solid</NAME></IMAGE></WIM>`)
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wim")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()
	if !w.Info().Solid {
		t.Error("expected solid for LZMS")
	}
	if len(w.Images()) != 1 {
		t.Errorf("got %d images, want 1", len(w.Images()))
	}

	if _, err := Open(filepath.Join(dir, "missing.wim")); err == nil {
		t.Error("expected error opening missing file")
	}
}

func TestArchAndUTF16Helpers(t *testing.T) {
	for code, want := range map[int]string{0: "x86", 5: "arm", 6: "ia64", 9: "x64", 12: "arm64", 99: "arch(99)"} {
		if got := archName(code); got != want {
			t.Errorf("archName(%d) = %q, want %q", code, got, want)
		}
	}
	// Odd-length / non-UTF16 input is returned unchanged.
	if got := decodeUTF16([]byte{0x41, 0x42, 0x43}); got != "ABC" {
		t.Errorf("decodeUTF16(odd) = %q", got)
	}
}
