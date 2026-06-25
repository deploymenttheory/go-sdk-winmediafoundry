package wim

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"

	internallzx "github.com/deploymenttheory/go-sdk-winmediafoundry/pkg/wim/lzx"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/pkg/wim/xpress"
)

// wimChunkSize is the uncompressed chunk size for compressed WIM resources. The
// 32 KiB default is what the Windows boot manager and Setup expect and what the
// reader assumes from the header chunk-size field.
const wimChunkSize = 32768

// writeBlobData writes content to the WIM as a resource. When the writer has a
// compression algorithm configured it stores the resource in the chunked
// compressed layout (a table of chunk start offsets followed by independently
// compressed 32 KiB chunks), exactly as chunkedReader expects; otherwise it
// writes the bytes verbatim. It returns the on-disk (compressed) size and the
// resource-descriptor flag bits to record in the blob table.
//
// A chunk that does not shrink is stored verbatim and signalled, per the WIM
// format, by its on-disk size equalling its uncompressed size — which is exactly
// how the reader decides to skip decompression for that chunk.
func (w *Writer) writeBlobData(content []byte) (onDisk int64, flags byte, err error) {
	if w.comp == CompressionNone || len(content) == 0 {
		if err := w.write(content); err != nil {
			return 0, 0, err
		}
		return int64(len(content)), 0, nil
	}

	nchunks := (len(content) + wimChunkSize - 1) / wimChunkSize
	chunks := make([][]byte, nchunks)

	// Chunks are independent; compress them across all CPUs. The XPRESS pass is
	// CPU-bound and otherwise bottlenecks large ISO builds on a single core.
	workers := runtime.NumCPU()
	if workers > nchunks {
		workers = nchunks
	}
	var (
		next int32 = -1
		wg   sync.WaitGroup
		mu   sync.Mutex
		cerr error
	)
	wg.Add(workers)
	for x := 0; x < workers; x++ {
		go func() {
			defer wg.Done()
			for {
				i := int(atomic.AddInt32(&next, 1))
				if i >= nchunks {
					return
				}
				start := i * wimChunkSize
				end := start + wimChunkSize
				if end > len(content) {
					end = len(content)
				}
				raw := content[start:end]
				c, e := compressChunk(w.comp, raw)
				if e != nil {
					mu.Lock()
					if cerr == nil {
						cerr = e
					}
					mu.Unlock()
					return
				}
				if len(c) >= len(raw) {
					c = raw // incompressible chunk: store verbatim
				}
				chunks[i] = c
			}
		}()
	}
	wg.Wait()
	if cerr != nil {
		return 0, 0, cerr
	}

	// Chunk table: start offset (relative to the end of the table) of chunks
	// 1..n-1; chunk 0 implicitly follows the table. 4-byte entries unless the
	// resource's uncompressed size needs 64 bits.
	entrySize := 4
	if len(content) > 0xffffffff {
		entrySize = 8
	}
	table := make([]byte, (nchunks-1)*entrySize)
	off := 0
	for i := 0; i < nchunks-1; i++ {
		off += len(chunks[i])
		if entrySize == 4 {
			binary.LittleEndian.PutUint32(table[i*4:], uint32(off))
		} else {
			binary.LittleEndian.PutUint64(table[i*8:], uint64(off))
		}
	}

	if err := w.write(table); err != nil {
		return 0, 0, err
	}
	total := int64(len(table))
	for _, c := range chunks {
		if err := w.write(c); err != nil {
			return 0, 0, err
		}
		total += int64(len(c))
	}
	return total, resFlagCompressed, nil
}

// compressChunk compresses one chunk with the given algorithm.
func compressChunk(comp Compression, raw []byte) ([]byte, error) {
	switch comp {
	case CompressionXPRESS:
		return xpress.Compress(raw)
	case CompressionLZX:
		return internallzx.Compress(raw)
	default:
		return nil, fmt.Errorf("wim: unsupported write compression %s", comp)
	}
}

// headerCompression returns the WIM header flag bits and chunk-size field for
// the writer's configured compression (both zero when uncompressed).
func (w *Writer) headerCompression() (flags uint32, chunkSize uint32) {
	switch w.comp {
	case CompressionXPRESS:
		return flagCompressed | flagCompressXP, wimChunkSize
	case CompressionLZX:
		return flagCompressed | flagCompressLZX, wimChunkSize
	default:
		return 0, 0
	}
}
