package wim

import (
	"encoding/binary"
	"fmt"

	"github.com/deploymenttheory/winmediafoundry/pkg/wim/lzms"
)

// solidResourceMagic marks an offset-table entry as a solid (packed) resource
// descriptor rather than a blob: its uncompressed_size field holds this value.
const solidResourceMagic = 0x100000000

// altHeaderSize is the size of a solid resource's alternate chunk-table header
// (res_usize uint64, chunk_size uint32, compression_format uint32).
const altHeaderSize = 16

// solidResource is a packed resource holding many blobs as a single compressed
// stream, split into fixed-size chunks (the last possibly smaller).
type solidResource struct {
	w          *WIM
	uncompSize int64
	chunkSize  int64
	comp       Compression
	chunkOff   []int64 // file offset of each chunk; len = numChunks+1 (last is end)

	cacheIdx  int
	cacheData []byte
}

// newSolidResource reads a solid resource's alt header and chunk table.
func (w *WIM) newSolidResource(fileOffset, compSize int64) (*solidResource, error) {
	var hdr [altHeaderSize]byte
	if _, err := w.r.ReadAt(hdr[:], fileOffset); err != nil {
		return nil, fmt.Errorf("wim: read solid header: %w", err)
	}
	le := binary.LittleEndian
	uncompSize := int64(le.Uint64(hdr[0:8]))
	chunkSize := int64(le.Uint32(hdr[8:12]))
	compFormat := le.Uint32(hdr[12:16])
	if chunkSize <= 0 || uncompSize < 0 {
		return nil, fmt.Errorf("%w: bad solid header", errBadMetadata)
	}

	numChunks := int((uncompSize + chunkSize - 1) / chunkSize)
	table := make([]byte, numChunks*4)
	if _, err := w.r.ReadAt(table, fileOffset+altHeaderSize); err != nil {
		return nil, fmt.Errorf("wim: read solid chunk table: %w", err)
	}

	s := &solidResource{
		w:          w,
		uncompSize: uncompSize,
		chunkSize:  chunkSize,
		comp:       compressionFromFormat(compFormat),
		chunkOff:   make([]int64, numChunks+1),
		cacheIdx:   -1,
	}
	dataStart := fileOffset + altHeaderSize + int64(numChunks*4)
	s.chunkOff[0] = dataStart
	for i := range numChunks {
		s.chunkOff[i+1] = s.chunkOff[i] + int64(le.Uint32(table[i*4:]))
	}
	_ = compSize
	return s, nil
}

func compressionFromFormat(f uint32) Compression {
	switch f {
	case 1:
		return CompressionXPRESS
	case 2:
		return CompressionLZX
	case 3:
		return CompressionLZMS
	default:
		return CompressionNone
	}
}

func (s *solidResource) numChunks() int { return len(s.chunkOff) - 1 }

func (s *solidResource) chunkUncompSize(i int) int64 {
	if i < s.numChunks()-1 {
		return s.chunkSize
	}
	if rem := s.uncompSize % s.chunkSize; rem != 0 {
		return rem
	}
	return s.chunkSize
}

// decompressChunk returns the decompressed bytes of chunk i, caching the most
// recently used chunk so sequential reads from the same chunk are cheap.
func (s *solidResource) decompressChunk(i int) ([]byte, error) {
	if s.cacheIdx == i {
		return s.cacheData, nil
	}
	compLen := s.chunkOff[i+1] - s.chunkOff[i]
	comp := make([]byte, compLen)
	if _, err := s.w.r.ReadAt(comp, s.chunkOff[i]); err != nil {
		return nil, fmt.Errorf("wim: read solid chunk %d: %w", i, err)
	}
	uncompSize := s.chunkUncompSize(i)

	var out []byte
	switch s.comp {
	case CompressionNone:
		out = comp
	case CompressionLZMS:
		if compLen == uncompSize {
			// Within a compressed resource, a chunk whose stored size equals its
			// uncompressed size was stored uncompressed (LZMS did not shrink it);
			// its bytes are the data verbatim. Decompressing them as LZMS would
			// produce garbage. This mirrors the standalone-resource reader
			// (decompress.go).
			out = comp
		} else {
			dec, err := lzms.Decompress(comp, int(uncompSize))
			if err != nil {
				return nil, fmt.Errorf("wim: solid chunk %d: %w", i, err)
			}
			out = dec
		}
	default:
		return nil, fmt.Errorf("%w: solid %s", errCompressionUnsupported, s.comp)
	}

	s.cacheIdx = i
	s.cacheData = out
	return out, nil
}

// readAt returns size bytes of the uncompressed solid resource starting at the
// given uncompressed offset, decompressing only the chunks it spans.
func (s *solidResource) readAt(offset, size int64) ([]byte, error) {
	if offset < 0 || size < 0 || offset+size > s.uncompSize {
		return nil, fmt.Errorf("%w: solid read out of range", errBadMetadata)
	}
	out := make([]byte, size)
	pos := int64(0)
	for size > 0 {
		idx := int(offset / s.chunkSize)
		chunk, err := s.decompressChunk(idx)
		if err != nil {
			return nil, err
		}
		within := offset % s.chunkSize
		n := min(int64(len(chunk))-within, size)
		copy(out[pos:pos+n], chunk[within:within+n])
		pos += n
		offset += n
		size -= n
	}
	return out, nil
}

// blobLocation records where a blob's uncompressed bytes live: either inside a
// solid resource, or as a standalone (possibly compressed) WIM resource.
type blobLocation struct {
	solid  *solidResource
	offset int64 // uncompressed offset within the solid resource
	size   int64
	rd     resourceDescriptor // standalone resource (when solid == nil)
}
