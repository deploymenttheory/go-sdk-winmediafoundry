package cab

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// compression method (low byte of CFFOLDER.typeCompress)
const (
	compNone  = 0x00
	compMSZIP = 0x01
	compLZX   = 0x03
	compMask  = 0x000f
)

// CFHEADER flags
const (
	flagPrevCabinet    = 0x0001
	flagNextCabinet    = 0x0002
	flagReservePresent = 0x0004
)

// Sentinel errors returned by the cabinet reader.
var (
	errNotCabinet      = errors.New("cab: not a Microsoft Cabinet file")
	errTruncated       = errors.New("cab: truncated cabinet")
	errCorruptCabinet  = errors.New("cab: corrupt cabinet structure")
	errUnsupportedComp = errors.New("cab: unsupported compression")
	errFileNotFound    = errors.New("cab: file not found")
)

// File is a single file extracted from a cabinet.
type File struct {
	Name string
	Data []byte
}

// folder mirrors a CFFOLDER: where its CFDATA blocks start, how many there are,
// and the compression method used for the folder's data.
type folder struct {
	coffCabStart uint32
	cCFData      uint16
	typeCompress uint16
}

// fileHeader mirrors a CFFILE: a file's uncompressed size, its offset within the
// decompressed folder, the folder it belongs to, and its name.
type fileHeader struct {
	cbFile          uint32
	uoffFolderStart uint32
	iFolder         uint16
	name            string
}

// Extract decompresses every file in the cabinet held in data.
func Extract(data []byte) ([]File, error) {
	if len(data) < 36 || string(data[0:4]) != "MSCF" {
		return nil, errNotCabinet
	}
	le := binary.LittleEndian
	coffFiles := le.Uint32(data[16:20])
	cFolders := int(le.Uint16(data[26:28]))
	cFiles := int(le.Uint16(data[28:30]))
	flags := le.Uint16(data[30:32])

	off := 36
	var cbCFFolder, cbCFData int
	if flags&flagReservePresent != 0 {
		if off+4 > len(data) {
			return nil, fmt.Errorf("reserve header: %w", errTruncated)
		}
		cbCFHeader := int(le.Uint16(data[off : off+2]))
		cbCFFolder = int(data[off+2])
		cbCFData = int(data[off+3])
		off += 4 + cbCFHeader
	}
	// Skip prev/next cabinet name+info strings (we only support single cabs).
	for _, fl := range []uint16{flagPrevCabinet, flagNextCabinet} {
		if flags&fl != 0 {
			var err error
			if off, err = skipCString(data, off); err != nil { // cabinet name
				return nil, err
			}
			if off, err = skipCString(data, off); err != nil { // disk name
				return nil, err
			}
		}
	}

	folders := make([]folder, cFolders)
	for i := range cFolders {
		if off+8 > len(data) {
			return nil, fmt.Errorf("CFFOLDER: %w", errTruncated)
		}
		folders[i] = folder{
			coffCabStart: le.Uint32(data[off : off+4]),
			cCFData:      le.Uint16(data[off+4 : off+6]),
			typeCompress: le.Uint16(data[off+6 : off+8]),
		}
		off += 8 + cbCFFolder
	}

	off = int(coffFiles)
	files := make([]fileHeader, cFiles)
	for i := range cFiles {
		if off+16 > len(data) {
			return nil, fmt.Errorf("CFFILE: %w", errTruncated)
		}
		fh := fileHeader{
			cbFile:          le.Uint32(data[off : off+4]),
			uoffFolderStart: le.Uint32(data[off+4 : off+8]),
			iFolder:         le.Uint16(data[off+8 : off+10]),
		}
		name, noff, err := readCString(data, off+16)
		if err != nil {
			return nil, err
		}
		fh.name = name
		files[i] = fh
		off = noff
	}

	// Decompress each folder once, then slice out each file.
	folderData := make([][]byte, cFolders)
	for i := range folders {
		fd, err := decompressFolder(data, folders[i], cbCFData)
		if err != nil {
			return nil, fmt.Errorf("cab: folder %d: %w", i, err)
		}
		folderData[i] = fd
	}

	out := make([]File, 0, cFiles)
	for _, fh := range files {
		if int(fh.iFolder) >= len(folderData) {
			return nil, fmt.Errorf(
				"file %q references missing folder %d: %w",
				fh.name,
				fh.iFolder,
				errCorruptCabinet,
			)
		}
		fd := folderData[fh.iFolder]
		start := int(fh.uoffFolderStart)
		end := start + int(fh.cbFile)
		if start > len(fd) || end > len(fd) {
			return nil, fmt.Errorf(
				"file %q extends past folder data: %w",
				fh.name,
				errCorruptCabinet,
			)
		}
		out = append(out, File{Name: fh.name, Data: fd[start:end]})
	}
	return out, nil
}

// ExtractFile returns the decompressed contents of a single named file.
func ExtractFile(data []byte, name string) ([]byte, error) {
	files, err := Extract(data)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if f.Name == name {
			return f.Data, nil
		}
	}
	return nil, fmt.Errorf("file %q: %w", name, errFileNotFound)
}

// decompressFolder reads the folder's CFDATA blocks and decompresses them.
func decompressFolder(data []byte, fl folder, cbCFData int) ([]byte, error) {
	le := binary.LittleEndian
	off := int(fl.coffCabStart)

	blocks := make([][]byte, 0, fl.cCFData)
	uncompSizes := make([]int, 0, fl.cCFData)
	total := 0
	for range int(fl.cCFData) {
		if off+8 > len(data) {
			return nil, fmt.Errorf("CFDATA header: %w", errTruncated)
		}
		cbData := int(le.Uint16(data[off+4 : off+6]))
		cbUncomp := int(le.Uint16(data[off+6 : off+8]))
		dataStart := off + 8 + cbCFData
		if dataStart+cbData > len(data) {
			return nil, fmt.Errorf("CFDATA block: %w", errTruncated)
		}
		blocks = append(blocks, data[dataStart:dataStart+cbData])
		uncompSizes = append(uncompSizes, cbUncomp)
		total += cbUncomp
		off = dataStart + cbData
	}

	switch fl.typeCompress & compMask {
	case compNone:
		out := make([]byte, 0, total)
		for _, b := range blocks {
			out = append(out, b...)
		}
		return out, nil
	case compMSZIP:
		return mszipDecompress(blocks, uncompSizes)
	case compLZX:
		windowBits := int((fl.typeCompress >> 8) & 0x1f)
		return lzxDecompress(blocks, uncompSizes, windowBits)
	default:
		return nil, fmt.Errorf("0x%04x: %w", fl.typeCompress, errUnsupportedComp)
	}
}

// skipCString advances past a NUL-terminated string and returns the offset of
// the byte after the terminator.
func skipCString(data []byte, off int) (int, error) {
	_, n, err := readCString(data, off)
	return n, err
}

// readCString reads a NUL-terminated string starting at off and returns it along
// with the offset of the byte after the terminator.
func readCString(data []byte, off int) (string, int, error) {
	end := off
	for end < len(data) && data[end] != 0 {
		end++
	}
	if end >= len(data) {
		return "", 0, fmt.Errorf("string: %w", errTruncated)
	}
	return string(data[off:end]), end + 1, nil
}
