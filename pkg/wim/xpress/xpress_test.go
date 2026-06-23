package xpress

import (
	"bytes"
	"testing"
)

// lensTable builds the 256-byte XPRESS codeword-length header from a
// symbol→length map (each byte packs two 4-bit lengths).
func lensTable(set map[int]int) []byte {
	t := make([]byte, lensBytes)
	for sym, l := range set {
		if sym%2 == 0 {
			t[sym/2] |= byte(l)
		} else {
			t[sym/2] |= byte(l) << 4
		}
	}
	return t
}

// TestDecompressLiterals decodes a hand-built chunk with the canonical code
// A=1 bit ("0"), B=2 bits ("10"), C=2 bits ("11"); the bitstream "0 10 11"
// packs MSB-first into the little-endian word 0x5800.
func TestDecompressLiterals(t *testing.T) {
	chunk := append(lensTable(map[int]int{'A': 1, 'B': 2, 'C': 2}), 0x00, 0x58)
	got, err := Decompress(chunk, 3)
	if err != nil {
		t.Fatalf("Decompress: %v", err)
	}
	if string(got) != "ABC" {
		t.Fatalf("got %q, want ABC", got)
	}
}

// TestDecompressMatch decodes A, B, then a match (symbol 273: length header 1,
// log2(offset)=1) with one offset bit 0 → offset 2, length 4, copying "ABAB".
func TestDecompressMatch(t *testing.T) {
	chunk := append(lensTable(map[int]int{'A': 1, 'B': 2, 273: 2}), 0x00, 0x58)
	got, err := Decompress(chunk, 6)
	if err != nil {
		t.Fatalf("Decompress: %v", err)
	}
	if string(got) != "ABABAB" {
		t.Fatalf("got %q, want ABABAB", got)
	}
}

// TestDecompressU16Length exercises the length==270 path, where the length is
// read as a raw little-endian u16. After the symbol word the offset's
// ensure(16) consumes the next word, so the extension byte and u16 follow it.
func TestDecompressU16Length(t *testing.T) {
	lens := lensTable(map[int]int{'A': 1, 257: 2, 271: 2})
	// symword 0x6000 ('A' then 271) | offword 0x0000 | ext 0xff | u16 = 20.
	chunk := append(lens, 0x00, 0x60, 0x00, 0x00, 0xff, 0x14, 0x00)
	got, err := Decompress(chunk, 24)
	if err != nil {
		t.Fatalf("Decompress: %v", err)
	}
	if !bytes.Equal(got, bytes.Repeat([]byte("A"), 24)) {
		t.Fatalf("got %q (len %d), want 24 A's", got, len(got))
	}
}

func TestDecompressErrors(t *testing.T) {
	if _, err := Decompress(make([]byte, 10), 4); err == nil {
		t.Error("expected error for short input (no length table)")
	}
	// Sole length-1 codeword is the match symbol 273, so the all-zero bitstream
	// decodes it first — a match against empty output, which must error.
	chunk := append(lensTable(map[int]int{273: 1}), 0x00, 0x00)
	if _, err := Decompress(chunk, 8); err == nil {
		t.Error("expected error for out-of-range match")
	}
}

// TestDecompressLongMatch exercises the length>=15 byte-extension path: symbol
// 256+15 = 271 (length header 0xf, log2 offset 0), then an offset bit and an
// extension byte read from the raw byte stream.
func TestDecompressLongMatch(t *testing.T) {
	// Code: 'A'=1 ("0"), filler 257=2 ("10"), match 271=2 ("11"). Canonical
	// assignment orders length-2 symbols by value, so 257<271 gives 271 the
	// codeword "11".
	lens := lensTable(map[int]int{'A': 1, 257: 2, 271: 2})
	// Bits: 'A'(0) then match 271 ("11") -> word 0x6000. log2_offset=0 (offset
	// 1), length header 0xf, then one raw extension byte = 5 -> length
	// 0xf+5 = 20, +3 = 23.
	chunk := append(lens, 0x00, 0x60, 0x05)
	got, err := Decompress(chunk, 24) // 'A' + 23 copies of 'A'
	if err != nil {
		t.Fatalf("Decompress: %v", err)
	}
	want := bytes.Repeat([]byte("A"), 24)
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q (len %d), want 24 A's", got, len(got))
	}
}

func TestBuildDecodeTableOversubscribed(t *testing.T) {
	var lens [numSymbols]byte
	// Three 1-bit codewords cannot coexist (over-subscribed).
	lens['A'], lens['B'], lens['C'] = 1, 1, 1
	if _, ok := buildDecodeTable(lens); ok {
		t.Error("expected over-subscribed code to be rejected")
	}
}

// TestRoundTripViaTable sanity-checks that a complete code over all 256 literals
// plus the match alphabet builds successfully (every literal at length 9 makes a
// complete code: 512 symbols * 2^(15-9) = 2^15).
func TestCompleteByteCode(t *testing.T) {
	var lens [numSymbols]byte
	for i := range lens {
		lens[i] = 9 // 512 codewords of length 9 form a complete code
	}
	table, ok := buildDecodeTable(lens)
	if !ok {
		t.Fatal("expected complete code")
	}
	if !bytes.Equal([]byte{byte(table[0] & 0xf)}, []byte{9}) {
		t.Errorf("entry length = %d, want 9", table[0]&0xf)
	}
}
