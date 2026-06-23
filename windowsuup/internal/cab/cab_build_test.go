package cab

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

// buildUncompressedCab assembles a minimal single-file, single-folder cabinet
// with no compression, optionally including a (zero-length) reserve area.
func buildUncompressedCab(name string, content []byte, withReserve bool) []byte {
	le := binary.LittleEndian
	nameBytes := append([]byte(name), 0)

	reserveLen := 0
	flags := uint16(0)
	if withReserve {
		reserveLen = 4 // cbCFHeader(2)+cbCFFolder(1)+cbCFData(1), all zero
		flags = flagReservePresent
	}

	coffFolder := 36 + reserveLen
	coffFile := coffFolder + 8
	coffData := coffFile + 16 + len(nameBytes)
	total := coffData + 8 + len(content)

	buf := make([]byte, total)
	copy(buf, "MSCF")
	le.PutUint32(buf[8:], uint32(total))     // cbCabinet
	le.PutUint32(buf[16:], uint32(coffFile)) // coffFiles
	buf[24], buf[25] = 3, 1                   // version
	le.PutUint16(buf[26:], 1)                 // cFolders
	le.PutUint16(buf[28:], 1)                 // cFiles
	le.PutUint16(buf[30:], flags)             // flags
	// reserve area (4 bytes of zeros) sits at offset 36 when present.

	// CFFOLDER
	le.PutUint32(buf[coffFolder:], uint32(coffData)) // coffCabStart
	le.PutUint16(buf[coffFolder+4:], 1)              // cCFData
	le.PutUint16(buf[coffFolder+6:], 0)              // typeCompress = none

	// CFFILE
	le.PutUint32(buf[coffFile:], uint32(len(content))) // cbFile
	le.PutUint32(buf[coffFile+4:], 0)                  // uoffFolderStart
	le.PutUint16(buf[coffFile+8:], 0)                  // iFolder
	copy(buf[coffFile+16:], nameBytes)                 // name

	// CFDATA
	le.PutUint16(buf[coffData+4:], uint16(len(content))) // cbData
	le.PutUint16(buf[coffData+6:], uint16(len(content))) // cbUncomp
	copy(buf[coffData+8:], content)

	return buf
}

func TestExtractUncompressedRoundTrip(t *testing.T) {
	content := []byte("hello from an uncompressed cabinet")
	for _, withReserve := range []bool{false, true} {
		cabBytes := buildUncompressedCab("note.txt", content, withReserve)
		files, err := Extract(cabBytes)
		if err != nil {
			t.Fatalf("reserve=%v: Extract: %v", withReserve, err)
		}
		if len(files) != 1 || files[0].Name != "note.txt" || !bytes.Equal(files[0].Data, content) {
			t.Fatalf("reserve=%v: unexpected files %+v", withReserve, files)
		}
	}
}

func TestExtractWithPrevCabinet(t *testing.T) {
	le := binary.LittleEndian
	content := []byte("spanned-set file body")
	prev := append([]byte("prev.cab"), 0)
	disk := append([]byte("disk1"), 0)
	nameBytes := append([]byte("p.txt"), 0)

	coffFolder := 36 + len(prev) + len(disk)
	coffFile := coffFolder + 8
	coffData := coffFile + 16 + len(nameBytes)
	total := coffData + 8 + len(content)

	buf := make([]byte, total)
	copy(buf, "MSCF")
	le.PutUint32(buf[8:], uint32(total))
	le.PutUint32(buf[16:], uint32(coffFile))
	buf[24], buf[25] = 3, 1
	le.PutUint16(buf[26:], 1)               // cFolders
	le.PutUint16(buf[28:], 1)               // cFiles
	le.PutUint16(buf[30:], flagPrevCabinet) // prev cabinet present
	copy(buf[36:], prev)
	copy(buf[36+len(prev):], disk)

	le.PutUint32(buf[coffFolder:], uint32(coffData)) // coffCabStart
	le.PutUint16(buf[coffFolder+4:], 1)              // cCFData
	le.PutUint16(buf[coffFolder+6:], 0)              // typeCompress none

	le.PutUint32(buf[coffFile:], uint32(len(content))) // cbFile
	copy(buf[coffFile+16:], nameBytes)

	le.PutUint16(buf[coffData+4:], uint16(len(content)))
	le.PutUint16(buf[coffData+6:], uint16(len(content)))
	copy(buf[coffData+8:], content)

	files, err := Extract(buf)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(files) != 1 || !bytes.Equal(files[0].Data, content) {
		t.Fatalf("unexpected files %+v", files)
	}
}

func TestExtractFileExceedsFolder(t *testing.T) {
	cabBytes := buildUncompressedCab("x", []byte("abc"), false)
	// Inflate the CFFILE cbFile so the file claims more data than the folder has.
	le := binary.LittleEndian
	coffFile := int(le.Uint32(cabBytes[16:20]))
	le.PutUint32(cabBytes[coffFile:], 0xffff)
	if _, err := Extract(cabBytes); !errors.Is(err, errCorruptCabinet) {
		t.Fatalf("got %v, want errCorruptCabinet", err)
	}
}

func TestDecompressFolderUnsupported(t *testing.T) {
	// One CFDATA block, Quantum compression (method 2) — unsupported.
	data := make([]byte, 8+4)
	binary.LittleEndian.PutUint16(data[4:], 4) // cbData
	binary.LittleEndian.PutUint16(data[6:], 4) // cbUncomp
	fl := folder{coffCabStart: 0, cCFData: 1, typeCompress: 0x0002}
	if _, err := decompressFolder(data, fl, 0); !errors.Is(err, errUnsupportedComp) {
		t.Fatalf("got %v, want errUnsupportedComp", err)
	}
}

func TestDecompressFolderTruncated(t *testing.T) {
	fl := folder{coffCabStart: 0, cCFData: 2, typeCompress: 0} // claims 2 blocks
	if _, err := decompressFolder([]byte{0, 0, 0, 0}, fl, 0); !errors.Is(err, errTruncated) {
		t.Fatalf("got %v, want errTruncated", err)
	}
}

func TestDecompressFolderNone(t *testing.T) {
	content := []byte("raw store bytes")
	data := make([]byte, 8+len(content))
	binary.LittleEndian.PutUint16(data[4:], uint16(len(content)))
	binary.LittleEndian.PutUint16(data[6:], uint16(len(content)))
	copy(data[8:], content)
	fl := folder{coffCabStart: 0, cCFData: 1, typeCompress: compNone}
	out, err := decompressFolder(data, fl, 0)
	if err != nil || !bytes.Equal(out, content) {
		t.Fatalf("decompressFolder(none) = %q, %v", out, err)
	}
}
