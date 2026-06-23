package wim

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"
	"unicode/utf16"
)

// File attribute bits used to classify directory entries.
const (
	attrDirectory    = 0x00000010
	attrReparsePoint = 0x00000400
)

const (
	direntrySize    = 102 // 8-byte length prefix + 94-byte fixed dentry
	streamentrySize = 38  // 8-byte length prefix + 30-byte fixed stream entry
)

// File is a file or directory within a WIM image.
type File struct {
	Name           string
	ShortName      string
	Attributes     uint32
	Size           int64
	Hash           [20]byte
	CreationTime   time.Time
	LastAccessTime time.Time
	LastWriteTime  time.Time

	children []*File
}

// IsDir reports whether the entry is a directory (and not a directory reparse
// point).
func (f *File) IsDir() bool {
	return f.Attributes&(attrDirectory|attrReparsePoint) == attrDirectory
}

// Children returns a directory's entries (nil for non-directories).
func (f *File) Children() []*File { return f.children }

// Walk calls fn for f and every descendant, depth-first. The path is built from
// entry names joined with "/".
func (f *File) Walk(fn func(path string, file *File)) {
	f.walk("", fn)
}

func (f *File) walk(prefix string, fn func(string, *File)) {
	path := f.Name
	if prefix != "" {
		path = prefix + "/" + f.Name
	}
	fn(path, f)
	for _, c := range f.children {
		c.walk(path, fn)
	}
}

var errBadMetadata = errors.New("wim: corrupt image metadata")

// filetimeToTime converts a Windows FILETIME (100-ns intervals since 1601) to a
// time.Time. A zero FILETIME maps to the zero time.Time (an unset timestamp); it
// would otherwise decode to a year-1601 time whose UnixNano overflows int64.
func filetimeToTime(low, high uint32) time.Time {
	ft := int64(high)<<32 | int64(low)
	if ft == 0 {
		return time.Time{}
	}
	return time.Unix(0, (ft-116444736000000000)*100).UTC()
}

// parseImageMetadata parses a decompressed image metadata resource (a security
// descriptor table followed by the directory-entry tree) into the image's root
// directory.
func parseImageMetadata(meta []byte) (*File, error) {
	rootOffset, err := securityTableSize(meta)
	if err != nil {
		return nil, err
	}
	roots, err := readDir(meta, rootOffset)
	if err != nil {
		return nil, err
	}
	if len(roots) != 1 {
		return nil, fmt.Errorf("%w: expected 1 root entry, got %d", errBadMetadata, len(roots))
	}
	return roots[0], nil
}

// securityTableSize returns the byte length of the leading security-descriptor
// table, which is where the root directory tree begins.
func securityTableSize(meta []byte) (int64, error) {
	if len(meta) < 8 {
		return 0, errBadMetadata
	}
	totalLength := binary.LittleEndian.Uint32(meta[0:4])
	// The security data is at minimum its 8-byte header; Microsoft stores
	// TotalLength 0 when an image has no security descriptors.
	size := max(int64((totalLength+7)&^7), 8)
	if size > int64(len(meta)) {
		return 0, fmt.Errorf("%w: bad security table size %d", errBadMetadata, size)
	}
	return size, nil
}

// readDir reads the sibling entries beginning at offset, recursing into
// subdirectories.
func readDir(meta []byte, offset int64) ([]*File, error) {
	var entries []*File
	pos := offset
	for {
		if pos+8 > int64(len(meta)) {
			return nil, errBadMetadata
		}
		length := int64(binary.LittleEndian.Uint64(meta[pos:]))
		if length == 0 {
			break // end-of-directory marker
		}
		f, total, err := readEntry(meta, pos, length)
		if err != nil {
			return nil, err
		}
		entries = append(entries, f)
		pos += total
	}
	return entries, nil
}

// readEntry parses a single directory entry (and its streams), recursing into a
// subdirectory if present. It returns the file and the total number of bytes the
// entry occupies.
func readEntry(meta []byte, pos, length int64) (*File, int64, error) {
	if length < direntrySize || pos+length > int64(len(meta)) {
		return nil, 0, errBadMetadata
	}
	le := binary.LittleEndian
	d := meta[pos:]

	// Dentry layout after the 8-byte length prefix: attributes(4)@8,
	// securityID(4)@12, subdirOffset(8)@16, unused(16)@24, creationTime(8)@40,
	// lastAccessTime(8)@48, lastWriteTime(8)@56, hash(20)@64, ...
	f := &File{
		Attributes:     le.Uint32(d[8:]),
		CreationTime:   filetimeToTime(le.Uint32(d[40:]), le.Uint32(d[44:])),
		LastAccessTime: filetimeToTime(le.Uint32(d[48:]), le.Uint32(d[52:])),
		LastWriteTime:  filetimeToTime(le.Uint32(d[56:]), le.Uint32(d[60:])),
	}
	copy(f.Hash[:], d[64:84])
	subdirOffset := int64(le.Uint64(d[16:]))
	streamCount := le.Uint16(d[96:])
	shortNameLen := int64(le.Uint16(d[98:]))
	fileNameLen := int64(le.Uint16(d[100:]))

	namesLen := fileNameLen + 2 + shortNameLen
	if direntrySize+namesLen > length {
		return nil, 0, errBadMetadata
	}
	names := d[direntrySize : direntrySize+namesLen]
	if fileNameLen > 0 {
		f.Name = decodeUTF16LE(names[:fileNameLen])
	}
	if shortNameLen > 0 {
		f.ShortName = decodeUTF16LE(names[fileNameLen+2 : fileNameLen+2+shortNameLen])
	}

	total := length

	// Alternate data streams follow the entry; the first unnamed stream is the
	// file's main data stream.
	for i := range int(streamCount) {
		name, hash, size, n, err := readStream(meta, pos+total)
		if err != nil {
			return nil, 0, err
		}
		total += n
		if i == 0 && name == "" {
			f.Hash = hash
			f.Size = size
		}
	}

	if f.IsDir() {
		children, err := readDir(meta, subdirOffset)
		if err != nil {
			return nil, 0, err
		}
		f.children = children
	}
	return f, total, nil
}

func readStream(meta []byte, pos int64) (name string, hash [20]byte, size int64, total int64, err error) {
	if pos+8 > int64(len(meta)) {
		return "", hash, 0, 0, errBadMetadata
	}
	length := int64(binary.LittleEndian.Uint64(meta[pos:]))
	if length < streamentrySize || pos+length > int64(len(meta)) {
		return "", hash, 0, 0, errBadMetadata
	}
	d := meta[pos:]
	copy(hash[:], d[16:36])
	nameLen := int64(binary.LittleEndian.Uint16(d[36:]))
	if streamentrySize+nameLen > length {
		return "", hash, 0, 0, errBadMetadata
	}
	if nameLen > 0 {
		name = decodeUTF16LE(d[streamentrySize : streamentrySize+nameLen])
	}
	return name, hash, 0, length, nil
}

func decodeUTF16LE(b []byte) string {
	u16 := make([]uint16, len(b)/2)
	for i := range u16 {
		u16[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return string(utf16.Decode(u16))
}

// readMetadataResource fully decompresses an image metadata resource.
func (w *WIM) readMetadataResource(rd resourceDescriptor) ([]byte, error) {
	if !rd.compressed() {
		return w.readResourceRaw(rd)
	}
	rc, err := w.resourceReader(rd)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}
