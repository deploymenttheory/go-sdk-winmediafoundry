package cab

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func TestBuildLZXTable(t *testing.T) {
	// A complete 2-symbol code (each 1 bit) fills the whole table.
	table := make([]uint16, lzxDecodeSize)
	if !buildLZXTable([]byte{1, 1}, table) {
		t.Fatal("expected complete code to build")
	}
	if table[0] != 0 || table[lzxDecodeSize-1] != 1 {
		t.Fatalf("unexpected table contents: [0]=%d [max]=%d", table[0], table[lzxDecodeSize-1])
	}

	// Over-subscribed lengths (three 1-bit codes need >1.0 of the space).
	if buildLZXTable([]byte{1, 1, 1}, table) {
		t.Fatal("expected over-subscribed code to fail")
	}
}

func TestDecodeE8(t *testing.T) {
	// 0xE8 followed by a small positive absolute target within filesize is
	// rewritten to a relative target (abs - currentPtr).
	const filesize = 1000
	b := make([]byte, 16)
	b[2] = 0xe8
	binary.LittleEndian.PutUint32(b[3:7], 500) // abs target at offset 2
	decodeE8(b, 0, filesize)
	got := int32(binary.LittleEndian.Uint32(b[3:7]))
	if want := int32(500 - (0 + 2)); got != want {
		t.Fatalf("E8 translation = %d, want %d", got, want)
	}

	// filesize 0 disables translation.
	b2 := make([]byte, 16)
	b2[0] = 0xe8
	binary.LittleEndian.PutUint32(b2[1:5], 500)
	decodeE8(b2, 0, 0)
	if binary.LittleEndian.Uint32(b2[1:5]) != 500 {
		t.Fatal("E8 should be a no-op when filesize is 0")
	}

	// Buffers shorter than 10 bytes are left untouched.
	short := []byte{0xe8, 1, 2, 3}
	cp := append([]byte{}, short...)
	decodeE8(short, 0, filesize)
	if !bytes.Equal(short, cp) {
		t.Fatal("E8 should not touch buffers shorter than 10 bytes")
	}
}

func TestLZXUnsupportedWindow(t *testing.T) {
	if _, err := lzxDecompress([][]byte{{0}}, []int{0}, 14); !errors.Is(err, errUnsupportedWindow) {
		t.Fatalf("got %v, want errUnsupportedWindow", err)
	}
}

// --- minimal LZX bit-stream writer for crafting test inputs ---

func appendBits(bits []int, val, n int) []int {
	for i := n - 1; i >= 0; i-- {
		bits = append(bits, (val>>i)&1)
	}
	return bits
}

// packLZXWords packs MSB-first bits into 16-bit little-endian words, matching
// the order the decoder reads them.
func packLZXWords(bits []int) []byte {
	for len(bits)%16 != 0 {
		bits = append(bits, 0)
	}
	var out []byte
	for i := 0; i < len(bits); i += 16 {
		var w uint16
		for j := 0; j < 16; j++ {
			w = w<<1 | uint16(bits[i+j])
		}
		out = append(out, byte(w), byte(w>>8))
	}
	return out
}

// TestLZXUncompressedBlock crafts a single uncompressed LZX block and checks it
// round-trips, exercising the uncompressed code path (readBlockHeader +
// copyUncompressed).
func TestLZXUncompressedBlock(t *testing.T) {
	data := []byte("uncompressed LZX block payload, raw bytes copied verbatim!!")
	n := len(data)

	var bits []int
	bits = appendBits(bits, 0, 1)        // E8 header bit: no translation
	bits = appendBits(bits, 3, 3)        // block type: uncompressed
	bits = appendBits(bits, n>>8, 16)    // block size, high 16 bits
	bits = appendBits(bits, n&0xff, 8)   // block size, low 8 bits
	header := packLZXWords(bits)         // header + alignment padding

	lru := make([]byte, 12) // R0,R1,R2
	binary.LittleEndian.PutUint32(lru[0:4], 1)
	binary.LittleEndian.PutUint32(lru[4:8], 1)
	binary.LittleEndian.PutUint32(lru[8:12], 1)

	chunk := append(append(append([]byte{}, header...), lru...), data...)

	out, err := lzxDecompress([][]byte{chunk}, []int{n}, 15)
	if err != nil {
		t.Fatalf("lzxDecompress: %v", err)
	}
	if !bytes.Equal(out, data) {
		t.Fatalf("uncompressed round-trip mismatch:\n got %q\nwant %q", out, data)
	}
}

func TestLZXCorruptInput(t *testing.T) {
	// A verbatim block header followed by zeroed tree data decodes to an
	// invalid (empty) main tree, which must surface as an error.
	var bits []int
	bits = appendBits(bits, 0, 1)     // E8 header bit
	bits = appendBits(bits, 1, 3)     // block type: verbatim
	bits = appendBits(bits, 64>>8, 16)
	bits = appendBits(bits, 64&0xff, 8)
	chunk := packLZXWords(bits)
	chunk = append(chunk, make([]byte, 64)...) // zero tree/data bytes

	if _, err := lzxDecompress([][]byte{chunk}, []int{64}, 15); err == nil {
		t.Fatal("expected error decoding corrupt verbatim block")
	}
}
