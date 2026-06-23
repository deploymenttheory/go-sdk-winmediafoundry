package lzms

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkDecompressRealChunks measures LZMS decompression throughput on the
// real Windows 11 ESD metadata resource (the same fixture the correctness test
// uses). Reported as MB/s of decompressed output.
func BenchmarkDecompressRealChunks(b *testing.B) {
	const (
		chunk0Out = 131072
		chunk1Out = 75056
	)
	data, err := os.ReadFile(filepath.Join("..", "testdata", "lzms-metadata-resource.bin"))
	if err != nil {
		b.Fatalf("read fixture: %v", err)
	}
	ch1 := int(binary.LittleEndian.Uint32(data))
	chunk0 := data[4 : 4+ch1]
	chunk1 := data[4+ch1:]
	totalOut := int64(chunk0Out + chunk1Out)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := Decompress(chunk0, chunk0Out); err != nil {
			b.Fatal(err)
		}
		if _, err := Decompress(chunk1, chunk1Out); err != nil {
			b.Fatal(err)
		}
	}
	b.SetBytes(totalOut)
}
