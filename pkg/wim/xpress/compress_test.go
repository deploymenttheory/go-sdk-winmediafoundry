package xpress

import (
	"bytes"
	"math/rand"
	"testing"
)

// roundTrip compresses in, decompresses the result back to len(in) bytes, and
// asserts byte-for-byte equality. It returns the compressed length.
func roundTrip(t *testing.T, name string, in []byte) int {
	t.Helper()
	comp, err := Compress(in)
	if err != nil {
		t.Fatalf("%s: Compress: %v", name, err)
	}
	got, err := Decompress(comp, len(in))
	if err != nil {
		t.Fatalf("%s: Decompress: %v (compressed %d bytes)", name, err, len(comp))
	}
	if !bytes.Equal(got, in) {
		// Find first mismatch for a useful message.
		idx := -1
		for i := 0; i < len(in) && i < len(got); i++ {
			if got[i] != in[i] {
				idx = i
				break
			}
		}
		t.Fatalf("%s: round-trip mismatch: len(in)=%d len(got)=%d firstDiff=%d",
			name, len(in), len(got), idx)
	}
	return len(comp)
}

func TestCompressRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
	}{
		{"empty", nil},
		{"single", []byte{0x41}},
		{"twoBytes", []byte{0x00, 0xff}},
		{"shortText", []byte("hello, hello, hello, world!")},
	}
	for _, c := range cases {
		roundTrip(t, c.name, c.in)
	}
}

func TestCompressAllSame(t *testing.T) {
	// All-same-byte forces very long matches, exercising lengthCode==15 overflow
	// and lengths well past 270 (u16 length path).
	for _, n := range []int{3, 4, 17, 18, 100, 273, 274, 1000, 32768} {
		in := bytes.Repeat([]byte{0xAB}, n)
		roundTrip(t, "allSame", in)
	}
}

func TestCompressRepetitiveText(t *testing.T) {
	base := []byte("The quick brown fox jumps over the lazy dog. ")
	var b bytes.Buffer
	for b.Len() < 32768 {
		b.Write(base)
	}
	in := b.Bytes()[:32768]
	clen := roundTrip(t, "repetitiveText", in)
	t.Logf("repetitive text: %d -> %d bytes (%.1f%%)", len(in), clen,
		100*float64(clen)/float64(len(in)))
}

func TestCompressRandom(t *testing.T) {
	// Incompressible: must still round-trip. May not shrink.
	rng := rand.New(rand.NewSource(12345))
	for _, n := range []int{1, 100, 1024, 32768} {
		in := make([]byte, n)
		rng.Read(in)
		roundTrip(t, "random", in)
	}
}

func TestCompressLargeOffsets(t *testing.T) {
	// Build data with a unique-ish prefix followed by a repeat of an early
	// region from far away (> 16384), so log2Offset is large (up to 15).
	rng := rand.New(rand.NewSource(99))
	in := make([]byte, 40000)
	rng.Read(in[:20000])
	// Copy a chunk from offset 0 to offset ~20000+, creating a long match at a
	// large offset (~20000, log2Offset ~14-15).
	copy(in[20000:20000+8000], in[0:8000])
	rng.Read(in[28000:])
	roundTrip(t, "largeOffsets", in)
}

func TestCompressLengthOverflowEdges(t *testing.T) {
	// Construct matches of exactly the boundary lengths around the overflow
	// thresholds: matchLen 17 (code 14, no overflow), 18 (code 15, +0 byte),
	// 272 (code 269, one byte), 273 (code 270 == 0xff path, u16), and beyond.
	for _, matchLen := range []int{16, 17, 18, 100, 270, 271, 272, 273, 274, 500, 5000} {
		// Pattern: a literal seed, then a long run that becomes one big match.
		// Use a 2-byte alternating seed so the match starts after a couple of
		// literals (offset small, but length is what we want to stress).
		in := make([]byte, 0, matchLen+8)
		in = append(in, 'X', 'Y')
		for len(in) < matchLen+2 {
			in = append(in, 'Z')
		}
		roundTrip(t, "lenOverflow", in)
	}
}

func TestCompressRatioRepetitive(t *testing.T) {
	// A highly compressible 32768-byte input must shrink to well under half.
	in := bytes.Repeat([]byte("ABCDABCDABCDABCD"), 32768/16)
	if len(in) != 32768 {
		t.Fatalf("setup: got %d bytes", len(in))
	}
	clen := roundTrip(t, "ratio", in)
	if clen >= len(in)/2 {
		t.Fatalf("ratio: compressed %d bytes, want < %d (half)", clen, len(in)/2)
	}
	t.Logf("compressible 32K: %d -> %d bytes (%.2f%%)", len(in), clen,
		100*float64(clen)/float64(len(in)))
}

func TestCompressFullChunk(t *testing.T) {
	// A full 32768-byte chunk of mixed data.
	rng := rand.New(rand.NewSource(7))
	in := make([]byte, 32768)
	for i := range in {
		// Mostly repetitive with occasional noise so both literals and matches
		// appear.
		if rng.Intn(20) == 0 {
			in[i] = byte(rng.Intn(256))
		} else {
			in[i] = byte('a' + (i % 5))
		}
	}
	roundTrip(t, "fullChunk", in)
}

func TestCompressFuzzRoundTrip(t *testing.T) {
	// Many randomized inputs across sizes and entropy profiles to stress the
	// round-trip contract for arbitrary data.
	rng := rand.New(rand.NewSource(20260625))
	for iter := 0; iter < 400; iter++ {
		n := rng.Intn(40000)
		in := make([]byte, n)
		switch rng.Intn(4) {
		case 0: // full random
			rng.Read(in)
		case 1: // low-entropy: few distinct bytes
			alphabet := byte(1 + rng.Intn(5))
			for i := range in {
				in[i] = byte(rng.Intn(int(alphabet)))
			}
		case 2: // runs of repeated bytes
			i := 0
			for i < n {
				run := 1 + rng.Intn(400)
				v := byte(rng.Intn(256))
				for j := 0; j < run && i < n; j++ {
					in[i] = v
					i++
				}
			}
		case 3: // text-like repetition with noise
			base := []byte("the quick brown fox ")
			for i := range in {
				if rng.Intn(30) == 0 {
					in[i] = byte(rng.Intn(256))
				} else {
					in[i] = base[i%len(base)]
				}
			}
		}
		roundTrip(t, "fuzz", in)
	}
}

func TestCompressEvery256Chunk(t *testing.T) {
	// Exercise every literal byte value so all 256 literal symbols can appear.
	in := make([]byte, 0, 4096)
	for i := 0; i < 16; i++ {
		for b := 0; b < 256; b++ {
			in = append(in, byte(b))
		}
	}
	roundTrip(t, "everyByte", in)
}
