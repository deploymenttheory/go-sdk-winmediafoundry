package wim

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/Microsoft/go-winio/wim/lzx"
	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/wim/lzms"
)

// The chunked resource reader below is adapted from the MIT-licensed
// github.com/microsoft/go-winio (wim/decompress.go, Copyright (c) Microsoft
// Corp), which is build-constrained to windows/linux and hardcodes LZX with a
// 32 KiB chunk size. This version is cross-platform, takes the chunk size and
// compression type from the WIM header, dispatches per chunk to the appropriate
// decompressor (reusing go-winio's LZX decoder), and leaves XPRESS/LZMS hooks
// for this SDK's own decompressors.

var errCompressionUnsupported = errors.New("wim: compression algorithm not yet supported")

// chunkedReader decompresses a non-solid WIM resource, which is stored as a
// table of chunk offsets followed by independently-compressed chunks that each
// decompress to chunkSize bytes (the last possibly fewer).
type chunkedReader struct {
	r            io.ReaderAt
	comp         Compression
	chunkSize    int64
	originalSize int64
	chunkOffsets []int64 // absolute offsets within r of each chunk's data; +1 sentinel
	cur          int
	dec          io.ReadCloser
}

// newChunkedReader builds a reader over a compressed resource located at r
// (a SectionReader over the WIM scoped to the resource).
func newChunkedReader(r io.ReaderAt, comp Compression, chunkSize, compressedSize, originalSize int64) (*chunkedReader, error) {
	nchunks := (originalSize + chunkSize - 1) / chunkSize
	cr := &chunkedReader{
		r:            r,
		comp:         comp,
		chunkSize:    chunkSize,
		originalSize: originalSize,
	}

	// The chunk table holds the start offset of chunks 1..n-1 (chunk 0 starts
	// right after the table). Entries are 4 bytes if the resource is < 4 GiB,
	// else 8 bytes.
	entrySize := int64(4)
	if originalSize > 0xffffffff {
		entrySize = 8
	}
	tableSize := (nchunks - 1) * entrySize

	cr.chunkOffsets = make([]int64, nchunks+1)
	cr.chunkOffsets[0] = tableSize
	if nchunks > 1 {
		raw := make([]byte, tableSize)
		if _, err := r.ReadAt(raw, 0); err != nil {
			return nil, fmt.Errorf("wim: read chunk table: %w", err)
		}
		for i := int64(1); i < nchunks; i++ {
			var off int64
			if entrySize == 4 {
				off = int64(binary.LittleEndian.Uint32(raw[(i-1)*4:]))
			} else {
				off = int64(binary.LittleEndian.Uint64(raw[(i-1)*8:]))
			}
			cr.chunkOffsets[i] = tableSize + off
		}
	}
	cr.chunkOffsets[nchunks] = compressedSize

	if err := cr.openChunk(0); err != nil {
		return nil, err
	}
	return cr, nil
}

func (cr *chunkedReader) uncompressedChunkSize(n int) int64 {
	if int64(n) < int64(len(cr.chunkOffsets))-2 {
		return cr.chunkSize
	}
	rem := cr.originalSize % cr.chunkSize
	if rem == 0 {
		return cr.chunkSize
	}
	return rem
}

func (cr *chunkedReader) openChunk(n int) error {
	if cr.dec != nil {
		_ = cr.dec.Close()
		cr.dec = nil
	}
	if n >= len(cr.chunkOffsets)-1 {
		return io.EOF
	}
	cr.cur = n
	start := cr.chunkOffsets[n]
	compSize := cr.chunkOffsets[n+1] - start
	uncompSize := cr.uncompressedChunkSize(n)
	section := io.NewSectionReader(cr.r, start, compSize)

	if compSize == uncompSize {
		cr.dec = io.NopCloser(section)
		return nil
	}
	switch cr.comp {
	case CompressionLZX:
		d, err := lzx.NewReader(section, int(uncompSize))
		if err != nil {
			return fmt.Errorf("wim: lzx chunk %d: %w", n, err)
		}
		cr.dec = d
	case CompressionXPRESS:
		return fmt.Errorf("%w: XPRESS", errCompressionUnsupported)
	case CompressionLZMS:
		comp := make([]byte, compSize)
		if _, err := io.ReadFull(section, comp); err != nil {
			return fmt.Errorf("wim: read lzms chunk %d: %w", n, err)
		}
		dec, err := lzms.Decompress(comp, int(uncompSize))
		if err != nil {
			return fmt.Errorf("wim: lzms chunk %d: %w", n, err)
		}
		cr.dec = io.NopCloser(bytes.NewReader(dec))
	default:
		return fmt.Errorf("%w: %s", errCompressionUnsupported, cr.comp)
	}
	return nil
}

func (cr *chunkedReader) Read(p []byte) (int, error) {
	for {
		n, err := cr.dec.Read(p)
		if !errors.Is(err, io.EOF) {
			return n, err
		}
		if n > 0 {
			return n, nil
		}
		if err := cr.openChunk(cr.cur + 1); err != nil {
			return 0, err
		}
	}
}

func (cr *chunkedReader) Close() error {
	if cr.dec != nil {
		return cr.dec.Close()
	}
	return nil
}

// resourceReader returns a reader over a resource's decompressed bytes.
func (w *WIM) resourceReader(rd resourceDescriptor) (io.ReadCloser, error) {
	section := io.NewSectionReader(w.r, rd.Offset, rd.CompressedSize)
	if !rd.compressed() {
		return io.NopCloser(section), nil
	}
	if rd.solid() {
		return nil, fmt.Errorf("%w: solid resource", errCompressionUnsupported)
	}
	return newChunkedReader(section, w.compression(), int64(w.hdr.ChunkSize), rd.CompressedSize, rd.OriginalSize)
}
