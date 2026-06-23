package cab

import (
	"bytes"
	"compress/flate"
	"strings"
	"testing"
)

// makeMSZIPBlock builds a single MSZIP CFDATA payload: the "CK" signature
// followed by a raw DEFLATE stream of data (optionally using dict as the
// preset dictionary, as MSZIP does for non-first blocks).
func makeMSZIPBlock(t *testing.T, data, dict []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	var (
		w   *flate.Writer
		err error
	)
	if dict == nil {
		w, err = flate.NewWriter(&buf, flate.BestCompression)
	} else {
		w, err = flate.NewWriterDict(&buf, flate.BestCompression, dict)
	}
	if err != nil {
		t.Fatalf("flate writer: %v", err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return append([]byte{'C', 'K'}, buf.Bytes()...)
}

func TestMSZIPSingleBlock(t *testing.T) {
	data := []byte(strings.Repeat("the quick brown fox 0123456789 ", 64))
	block := makeMSZIPBlock(t, data, nil)

	out, err := mszipDecompress([][]byte{block}, []int{len(data)})
	if err != nil {
		t.Fatalf("mszipDecompress: %v", err)
	}
	if !bytes.Equal(out, data) {
		t.Fatalf("round-trip mismatch: got %d bytes, want %d", len(out), len(data))
	}
}

func TestMSZIPMultiBlockWithDict(t *testing.T) {
	d0 := []byte(strings.Repeat("alpha beta gamma delta ", 100))
	d1 := []byte(strings.Repeat("alpha beta gamma delta epsilon ", 100)) // shares prefix with d0

	b0 := makeMSZIPBlock(t, d0, nil)
	b1 := makeMSZIPBlock(t, d1, d0) // dictionary is the previous block's output

	out, err := mszipDecompress([][]byte{b0, b1}, []int{len(d0), len(d1)})
	if err != nil {
		t.Fatalf("mszipDecompress: %v", err)
	}
	want := append(append([]byte{}, d0...), d1...)
	if !bytes.Equal(out, want) {
		t.Fatalf("multi-block mismatch: got %d bytes, want %d", len(out), len(want))
	}
}

func TestMSZIPMissingSignature(t *testing.T) {
	if _, err := mszipDecompress([][]byte{{0x00, 0x01, 0x02}}, []int{3}); err == nil {
		t.Fatal("expected error for block missing CK signature")
	}
}

func TestMSZIPInvalidDeflate(t *testing.T) {
	// "CK" signature followed by an invalid DEFLATE stream.
	bad := []byte{'C', 'K', 0xff, 0xff, 0xff, 0xff}
	if _, err := mszipDecompress([][]byte{bad}, []int{16}); err == nil {
		t.Fatal("expected error for invalid DEFLATE payload")
	}
}
