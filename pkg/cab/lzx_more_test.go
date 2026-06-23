package cab

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// uncompressedChunk builds a single-block uncompressed LZX chunk for content.
func uncompressedChunk(content []byte) []byte {
	n := len(content)
	var bits []int
	bits = appendBits(bits, 0, 1)      // E8 header bit
	bits = appendBits(bits, 3, 3)      // uncompressed block
	bits = appendBits(bits, n>>8, 16)  // size high
	bits = appendBits(bits, n&0xff, 8) // size low
	header := packLZXWords(bits)

	lru := make([]byte, 12)
	binary.LittleEndian.PutUint32(lru[0:4], 1)
	binary.LittleEndian.PutUint32(lru[4:8], 1)
	binary.LittleEndian.PutUint32(lru[8:12], 1)
	return append(append(append([]byte{}, header...), lru...), content...)
}

// TestLZXUncompressedLarge uses a block larger than the read-ahead buffer so the
// remainder of the raw data is read from the underlying reader, not the buffer.
func TestLZXUncompressedLarge(t *testing.T) {
	content := bytes.Repeat([]byte("0123456789abcdef"), 400) // 6400 bytes > 4096 buffer
	out, err := lzxDecompress([][]byte{uncompressedChunk(content)}, []int{len(content)}, 16)
	if err != nil {
		t.Fatalf("lzxDecompress: %v", err)
	}
	if !bytes.Equal(out, content) {
		t.Fatalf("large uncompressed mismatch: got %d, want %d bytes", len(out), len(content))
	}
}

// lruBytes returns 12 bytes encoding R0=R1=R2=1.
func lruBytes() []byte {
	b := make([]byte, 12)
	binary.LittleEndian.PutUint32(b[0:4], 1)
	binary.LittleEndian.PutUint32(b[4:8], 1)
	binary.LittleEndian.PutUint32(b[8:12], 1)
	return b
}

// TestLZXTwoUncompressedBlocks crafts a chunk with two uncompressed blocks where
// the first has odd length, exercising the 16-bit realignment (unaligned) path
// between blocks.
func TestLZXTwoUncompressedBlocks(t *testing.T) {
	d0 := []byte("ABCDE") // 5 bytes (odd)
	d1 := []byte("FGHIJ") // 5 bytes

	// Block 0 header: E8 bit + type(uncompressed) + 24-bit size.
	var b0 []int
	b0 = appendBits(b0, 0, 1)
	b0 = appendBits(b0, 3, 3)
	b0 = appendBits(b0, len(d0)>>8, 16)
	b0 = appendBits(b0, len(d0)&0xff, 8)

	// Block 1 header: type + 24-bit size (no E8 bit; that is per-chunk).
	var b1 []int
	b1 = appendBits(b1, 3, 3)
	b1 = appendBits(b1, len(d1)>>8, 16)
	b1 = appendBits(b1, len(d1)&0xff, 8)

	chunk := append([]byte{}, packLZXWords(b0)...)
	chunk = append(chunk, lruBytes()...)
	chunk = append(chunk, d0...)
	chunk = append(chunk, 0) // odd-length realignment padding byte
	chunk = append(chunk, packLZXWords(b1)...)
	chunk = append(chunk, lruBytes()...)
	chunk = append(chunk, d1...)

	out, err := lzxDecompress([][]byte{chunk}, []int{len(d0) + len(d1)}, 15)
	if err != nil {
		t.Fatalf("lzxDecompress: %v", err)
	}
	if want := append(append([]byte{}, d0...), d1...); !bytes.Equal(out, want) {
		t.Fatalf("two-block mismatch: got %q want %q", out, want)
	}
}

// TestLZXUncompressedTruncated declares an uncompressed block but supplies too
// few bytes for the R0/R1/R2 header, exercising that error path.
func TestLZXUncompressedTruncated(t *testing.T) {
	var bits []int
	bits = appendBits(bits, 0, 1) // E8 header bit
	bits = appendBits(bits, 3, 3) // uncompressed block
	bits = appendBits(bits, 100>>8, 16)
	bits = appendBits(bits, 100&0xff, 8)
	chunk := packLZXWords(bits) // header only; no R0/R1/R2 bytes follow

	if _, err := lzxDecompress([][]byte{chunk}, []int{100}, 15); err == nil {
		t.Fatal("expected error for truncated uncompressed block header")
	}
}

// TestLZXInvalidBlockType uses an unrecognized block type, exercising the
// block-header error path.
func TestLZXInvalidBlockType(t *testing.T) {
	var bits []int
	bits = appendBits(bits, 0, 1) // E8 header bit
	bits = appendBits(bits, 0, 3) // block type 0 (invalid)
	bits = appendBits(bits, 32>>8, 16)
	bits = appendBits(bits, 32&0xff, 8)
	chunk := packLZXWords(bits)
	chunk = append(chunk, make([]byte, 32)...)

	if _, err := lzxDecompress([][]byte{chunk}, []int{32}, 15); err == nil {
		t.Fatal("expected error for invalid block type")
	}
}

// TestLZXTruncatedStream feeds a verbatim block header with too few bytes to read
// the trees, exercising the unexpected-EOF path.
func TestLZXTruncatedStream(t *testing.T) {
	var bits []int
	bits = appendBits(bits, 0, 1) // E8 header bit
	bits = appendBits(bits, 1, 3) // verbatim block
	bits = appendBits(bits, 256>>8, 16)
	bits = appendBits(bits, 256&0xff, 8)
	chunk := packLZXWords(bits) // header only; no tree bytes follow

	if _, err := lzxDecompress([][]byte{chunk}, []int{256}, 15); err == nil {
		t.Fatal("expected unexpected-EOF error on truncated stream")
	}
}
