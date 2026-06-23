// Package wim reads Windows Imaging Format (WIM) files, including the solid
// LZMS-compressed ESD variant distributed by Windows Update / the Media
// Creation Tool.
//
// It is pure Go and cross-platform. The container layer (header, resource/blob
// table, and the XML image catalog) needs no decompression, so images can be
// enumerated immediately; extracting an image's contents additionally requires
// the XPRESS/LZX/LZMS decompressors.
package wim

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// imageTag is the 8-byte magic at the start of every WIM file ("MSWIM\0\0\0").
var imageTag = [8]byte{'M', 'S', 'W', 'I', 'M', 0, 0, 0}

const (
	headerSize = 208

	// Header flag bits (dwFlags).
	flagCompressed   = 0x00000002
	flagSpanned      = 0x00000008
	flagCompressXP   = 0x00020000 // XPRESS
	flagCompressLZX  = 0x00040000 // LZX
	flagCompressLZMS = 0x00080000 // LZMS

	// Resource descriptor flag bits (high byte of size field).
	resFlagFree       = 0x01
	resFlagMetadata   = 0x02
	resFlagCompressed = 0x04
	resFlagSpanned    = 0x08 // spanned across split WIM parts
	resFlagSolid      = 0x10 // packed into a solid resource (newer ESD format)
)

// Compression identifies a WIM's resource compression algorithm.
type Compression int

const (
	CompressionNone Compression = iota
	CompressionXPRESS
	CompressionLZX
	CompressionLZMS
)

func (c Compression) String() string {
	switch c {
	case CompressionXPRESS:
		return "XPRESS"
	case CompressionLZX:
		return "LZX"
	case CompressionLZMS:
		return "LZMS"
	default:
		return "none"
	}
}

// resourceDescriptor locates a resource within the WIM and records its on-disk
// (possibly compressed) size, original size, and flags.
type resourceDescriptor struct {
	Offset         int64
	CompressedSize int64
	OriginalSize   int64
	Flags          byte
}

func (r resourceDescriptor) compressed() bool { return r.Flags&resFlagCompressed != 0 }
func (r resourceDescriptor) solid() bool      { return r.Flags&resFlagSolid != 0 }

// header mirrors the 208-byte WIM header (WIMHEADER_V1_PACKED).
type header struct {
	Version      uint32
	Flags        uint32
	ChunkSize    uint32
	GUID         [16]byte
	PartNumber   uint16
	TotalParts   uint16
	ImageCount   uint32
	OffsetTable  resourceDescriptor
	XMLData      resourceDescriptor
	BootMetadata resourceDescriptor
	BootIndex    uint32
	Integrity    resourceDescriptor
}

// Info summarizes a WIM's header.
type Info struct {
	Version     uint32
	Compression Compression
	ChunkSize   uint32
	ImageCount  int
	BootIndex   int
	PartNumber  int
	TotalParts  int
	Solid       bool
	GUID        [16]byte
}

var (
	errNotWIM      = errors.New("wim: not a WIM file (bad magic)")
	errShortHeader = errors.New("wim: file too small for header")
)

// WIM is an opened WIM/ESD file.
type WIM struct {
	r       io.ReaderAt
	closer  io.Closer
	hdr     header
	images  []ImageInfo
	xmlUTF8 string

	// Lazily-parsed offset/blob table.
	blobTableLoaded bool
	metadataRes     []resourceDescriptor      // per-image metadata, in image order
	blobs           map[[20]byte]blobLocation // file-content blobs by SHA-1
}

// Open opens the WIM/ESD file at path.
func Open(path string) (*WIM, error) {
	f, err := os.Open(path) //nolint:gosec // caller-provided path
	if err != nil {
		return nil, fmt.Errorf("wim: open %s: %w", path, err)
	}
	w, err := OpenReaderAt(f, mustSize(f))
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	w.closer = f
	return w, nil
}

func mustSize(f *os.File) int64 {
	if st, err := f.Stat(); err == nil {
		return st.Size()
	}
	return 0
}

// OpenReaderAt opens a WIM/ESD from r. size is the total file size (used for
// bounds checking); pass 0 if unknown.
func OpenReaderAt(r io.ReaderAt, size int64) (*WIM, error) {
	buf := make([]byte, headerSize)
	if _, err := r.ReadAt(buf, 0); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, errShortHeader
		}
		return nil, fmt.Errorf("wim: read header: %w", err)
	}

	var tag [8]byte
	copy(tag[:], buf[:8])
	if tag != imageTag {
		return nil, errNotWIM
	}

	hdr, err := parseHeader(buf)
	if err != nil {
		return nil, err
	}

	w := &WIM{r: r, hdr: hdr}
	if err := w.loadXML(size); err != nil {
		return nil, err
	}
	return w, nil
}

// Close closes the underlying file if Open created it.
func (w *WIM) Close() error {
	if w.closer != nil {
		return w.closer.Close()
	}
	return nil
}

// Info returns the WIM header summary.
func (w *WIM) Info() Info {
	return Info{
		Version:     w.hdr.Version,
		Compression: w.compression(),
		ChunkSize:   w.hdr.ChunkSize,
		ImageCount:  int(w.hdr.ImageCount),
		BootIndex:   int(w.hdr.BootIndex),
		PartNumber:  int(w.hdr.PartNumber),
		TotalParts:  int(w.hdr.TotalParts),
		// LZMS is only ever used for solid (packed) resources, so it implies a
		// solid WIM; the precise per-resource flag is read from the blob table.
		Solid: w.compression() == CompressionLZMS,
		GUID:  w.hdr.GUID,
	}
}

func (w *WIM) compression() Compression {
	switch {
	case w.hdr.Flags&flagCompressLZMS != 0:
		return CompressionLZMS
	case w.hdr.Flags&flagCompressLZX != 0:
		return CompressionLZX
	case w.hdr.Flags&flagCompressXP != 0:
		return CompressionXPRESS
	default:
		return CompressionNone
	}
}

// parseHeader decodes the 208-byte WIM header.
func parseHeader(b []byte) (header, error) {
	le := binary.LittleEndian
	var h header
	h.Version = le.Uint32(b[12:16])
	h.Flags = le.Uint32(b[16:20])
	h.ChunkSize = le.Uint32(b[20:24])
	copy(h.GUID[:], b[24:40])
	h.PartNumber = le.Uint16(b[40:42])
	h.TotalParts = le.Uint16(b[42:44])
	h.ImageCount = le.Uint32(b[44:48])
	h.OffsetTable = parseResource(b[48:72])
	h.XMLData = parseResource(b[72:96])
	h.BootMetadata = parseResource(b[96:120])
	h.BootIndex = le.Uint32(b[120:124])
	h.Integrity = parseResource(b[124:148])
	return h, nil
}

// parseResource decodes a 24-byte resource descriptor: an 8-byte little-endian
// value whose low 56 bits are the compressed size and high 8 bits are flags,
// followed by the 8-byte offset and 8-byte original size.
func parseResource(b []byte) resourceDescriptor {
	le := binary.LittleEndian
	sizeAndFlags := le.Uint64(b[0:8])
	return resourceDescriptor{
		CompressedSize: int64(sizeAndFlags & 0x00FFFFFFFFFFFFFF),
		Flags:          byte(sizeAndFlags >> 56),
		Offset:         int64(le.Uint64(b[8:16])),
		OriginalSize:   int64(le.Uint64(b[16:24])),
	}
}

// readResourceRaw reads a resource's raw (on-disk) bytes.
func (w *WIM) readResourceRaw(rd resourceDescriptor) ([]byte, error) {
	if rd.CompressedSize == 0 {
		return nil, nil
	}
	buf := make([]byte, rd.CompressedSize)
	if _, err := w.r.ReadAt(buf, rd.Offset); err != nil {
		return nil, fmt.Errorf("wim: read resource at %d: %w", rd.Offset, err)
	}
	return buf, nil
}
