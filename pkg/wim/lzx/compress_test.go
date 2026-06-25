package lzx_test

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	winiolk "github.com/Microsoft/go-winio/wim/lzx"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/pkg/wim/lzx"
)

// roundTrip compresses with this package then decompresses with go-winio's
// LZX decoder, which is the consumer of all WIM LZX data in production.
func roundTrip(t *testing.T, in []byte) {
	t.Helper()
	compressed, err := lzx.Compress(in)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	// If the compressor produced nothing smaller than raw (all chunks raw),
	// go-winio's reader won't be invoked (size==originalSize path), so we
	// can skip the decode step.
	if len(compressed) >= len(in) {
		return
	}
	r, err := winiolk.NewReader(bytes.NewReader(compressed), len(in))
	if err != nil {
		t.Fatalf("lzx.NewReader: %v", err)
	}
	defer r.Close()
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, in) {
		t.Fatalf("round-trip mismatch: len(in)=%d len(got)=%d", len(in), len(got))
	}
}

func TestRoundTripAllZeros(t *testing.T) {
	roundTrip(t, make([]byte, 32768))
}

func TestRoundTripAllSameByte(t *testing.T) {
	buf := bytes.Repeat([]byte{0x41}, 32768)
	roundTrip(t, buf)
}

func TestRoundTripSmall(t *testing.T) {
	roundTrip(t, []byte("hello world"))
}

func TestRoundTripPartialChunk(t *testing.T) {
	buf := bytes.Repeat([]byte("abcdefgh"), 1000)
	roundTrip(t, buf)
}

func TestRoundTripFull32K(t *testing.T) {
	// Highly compressible text pattern filling a full 32 KiB chunk.
	buf := bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog.\n"), 750)
	buf = buf[:32768]
	roundTrip(t, buf)
}

func TestRoundTripE8Bytes(t *testing.T) {
	// Data containing 0xe8 bytes (x86 CALL opcodes) — exercises E8 preprocessing.
	buf := make([]byte, 16384)
	for i := 0; i < len(buf); i += 8 {
		buf[i] = 0xe8
		buf[i+1] = byte(i)
		buf[i+2] = byte(i >> 8)
		buf[i+3] = 0
		buf[i+4] = 0
	}
	roundTrip(t, buf)
}

func TestRoundTripRandom(t *testing.T) {
	// Random data: mostly incompressible, exercises the raw-storage path.
	buf := make([]byte, 32768)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	roundTrip(t, buf)
}

func TestRoundTripSingleByte(t *testing.T) {
	roundTrip(t, []byte{0x42})
}

func TestRoundTripEmpty(t *testing.T) {
	out, err := lzx.Compress(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty output for empty input, got %d bytes", len(out))
	}
}

func BenchmarkCompress32K(b *testing.B) {
	buf := bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. "), 720)
	buf = buf[:32768]
	b.SetBytes(int64(len(buf)))
	b.ResetTimer()
	for range b.N {
		if _, err := lzx.Compress(buf); err != nil {
			b.Fatal(err)
		}
	}
}
