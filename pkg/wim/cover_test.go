package wim

import (
	"bytes"
	"crypto/sha1" //nolint:gosec // integrity check, not security
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCompressionString(t *testing.T) {
	for c, want := range map[Compression]string{
		CompressionNone: "none", CompressionXPRESS: "XPRESS",
		CompressionLZX: "LZX", CompressionLZMS: "LZMS",
	} {
		if got := c.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", c, got, want)
		}
	}
}

// TestResourceReaderLZMS builds a WIM whose header advertises LZMS with 128 KiB
// chunks, places a real LZMS-compressed metadata resource after the header, and
// reads it back through resourceReader, verifying the SHA-1.
func TestResourceReaderLZMS(t *testing.T) {
	const (
		resOrigSize = 206128
		wantSHA1    = "c6d0872da60175857d807d5714f49240b6adcff9"
	)
	comp, err := os.ReadFile(filepath.Join("testdata", "lzms-metadata-resource.bin"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	le := binary.LittleEndian
	hdr := make([]byte, headerSize)
	copy(hdr, imageTag[:])
	le.PutUint32(hdr[12:], 0x00000e00)                      // version
	le.PutUint32(hdr[16:], flagCompressLZMS|flagCompressed) // flags
	le.PutUint32(hdr[20:], 131072)                          // chunk size
	// XMLData (offset 72) left zero so loadXML is skipped.
	data := append(hdr, comp...)

	w, err := OpenReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("OpenReaderAt: %v", err)
	}
	if w.Info().Compression != CompressionLZMS {
		t.Fatalf("compression = %v", w.Info().Compression)
	}

	rd := resourceDescriptor{
		Offset:         headerSize,
		CompressedSize: int64(len(comp)),
		OriginalSize:   resOrigSize,
		Flags:          resFlagCompressed,
	}
	rc, err := w.resourceReader(rd)
	if err != nil {
		t.Fatalf("resourceReader: %v", err)
	}
	defer rc.Close()
	out, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	sum := sha1.Sum(out) //nolint:gosec
	if got := hex.EncodeToString(sum[:]); got != wantSHA1 {
		t.Fatalf("sha1 = %s, want %s", got, wantSHA1)
	}

	// An uncompressed resource reads straight through.
	rc2, err := w.resourceReader(resourceDescriptor{Offset: 0, CompressedSize: 8})
	if err != nil {
		t.Fatalf("resourceReader(uncompressed): %v", err)
	}
	_ = rc2.Close()
}

func TestLoadXMLCompressedUnsupported(t *testing.T) {
	le := binary.LittleEndian
	hdr := make([]byte, headerSize)
	copy(hdr, imageTag[:])
	le.PutUint32(hdr[16:], flagCompressLZMS|flagCompressed)
	le.PutUint32(hdr[20:], 131072)
	// XMLData: nonzero, compressed flag set -> unsupported.
	le.PutUint64(hdr[72:], uint64(resFlagCompressed)<<56|100) // size 100, compressed
	le.PutUint64(hdr[80:], headerSize)
	le.PutUint64(hdr[88:], 200)
	data := append(hdr, make([]byte, 200)...)

	if _, err := OpenReaderAt(bytes.NewReader(data), int64(len(data))); err == nil {
		t.Fatal("expected error for compressed XML catalog")
	}
}

func TestChunkedReaderTableError(t *testing.T) {
	// originalSize requires 2 chunks (a chunk table), but no table bytes exist.
	_, err := newChunkedReader(bytes.NewReader(nil), CompressionLZX, 8, 100, 16)
	if err == nil {
		t.Fatal("expected chunk-table read error")
	}
}

func TestOpenReaderAtErrors(t *testing.T) {
	// Read error: a reader that fails on ReadAt.
	if _, err := OpenReaderAt(failReaderAt{}, 0); err == nil {
		t.Fatal("expected read error")
	}
}

type failReaderAt struct{}

func (failReaderAt) ReadAt([]byte, int64) (int, error) { return 0, io.ErrClosedPipe }

func TestReadResourceRaw(t *testing.T) {
	w := &WIM{r: bytes.NewReader([]byte("0123456789"))}
	if got, _ := w.readResourceRaw(resourceDescriptor{CompressedSize: 0}); got != nil {
		t.Errorf("zero-size resource should read nil, got %v", got)
	}
	got, err := w.readResourceRaw(resourceDescriptor{Offset: 2, CompressedSize: 4})
	if err != nil || string(got) != "2345" {
		t.Errorf("readResourceRaw = %q, %v", got, err)
	}
	wbad := &WIM{r: failReaderAt{}}
	if _, err := wbad.readResourceRaw(resourceDescriptor{CompressedSize: 4}); err == nil {
		t.Error("expected read error")
	}
}

func TestLoadXMLMalformed(t *testing.T) {
	// XML resource that decodes to invalid XML must surface a parse error.
	bad := utf16le("this is < not & valid xml")
	le := binary.LittleEndian
	hdr := make([]byte, headerSize)
	copy(hdr, imageTag[:])
	le.PutUint64(hdr[72:], uint64(len(bad))) // XMLData size, uncompressed
	le.PutUint64(hdr[80:], headerSize)
	le.PutUint64(hdr[88:], uint64(len(bad)))
	data := append(hdr, bad...)

	if _, err := OpenReaderAt(bytes.NewReader(data), int64(len(data))); err == nil {
		t.Fatal("expected XML parse error")
	}
}

func TestChunkedReaderCloseNoDecoder(t *testing.T) {
	cr := &chunkedReader{}
	if err := cr.Close(); err != nil {
		t.Errorf("Close with no decoder: %v", err)
	}
}

func utf16le(s string) []byte {
	out := []byte{0xFF, 0xFE}
	for _, r := range s {
		out = append(out, byte(r), byte(r>>8))
	}
	return out
}
