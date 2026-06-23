package wim

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func align8(n int) int { return (n + 7) &^ 7 }

func u16le(s string) []byte {
	enc := utf16.Encode([]rune(s))
	b := make([]byte, len(enc)*2)
	for i, c := range enc {
		binary.LittleEndian.PutUint16(b[i*2:], c)
	}
	return b
}

// buildDentry builds one directory entry followed by its alternate data streams.
func buildDentry(attrs uint32, subdir int64, name, short string, streamNames []string) []byte {
	le := binary.LittleEndian
	nameB, shortB := u16le(name), u16le(short)
	namesLen := len(nameB) + 2 + len(shortB)
	size := align8(102 + namesLen)

	d := make([]byte, size)
	le.PutUint64(d[0:], uint64(size))
	le.PutUint32(d[8:], attrs)
	le.PutUint32(d[12:], 0xffffffff) // no security
	le.PutUint64(d[16:], uint64(subdir))
	le.PutUint16(d[96:], uint16(len(streamNames)))
	le.PutUint16(d[98:], uint16(len(shortB)))
	le.PutUint16(d[100:], uint16(len(nameB)))
	copy(d[102:], nameB)
	copy(d[102+len(nameB)+2:], shortB)

	for _, sn := range streamNames {
		snB := u16le(sn)
		slen := align8(38 + len(snB))
		s := make([]byte, slen)
		le.PutUint64(s[0:], uint64(slen))
		le.PutUint16(s[36:], uint16(len(snB)))
		copy(s[38:], snB)
		d = append(d, s...)
	}
	return d
}

func endMarker() []byte { return make([]byte, 8) }

// buildSyntheticMetadata builds a metadata resource: a root directory with two
// children — a file with a short name, and a file with one unnamed data stream.
func buildSyntheticMetadata() ([]byte, int64) {
	sec := make([]byte, 8) // security header: TotalLength 0

	child1 := buildDentry(0x20, 0, "file.txt", "FILE~1.TXT", nil)
	child2 := buildDentry(0x20, 0, "data.bin", "", []string{""}) // one unnamed stream
	childList := append(append(child1, child2...), endMarker()...)

	// root precedes its end marker, which precedes the child list.
	rootNoSubdir := buildDentry(attrDirectory, 0, "", "", nil)
	childOffset := int64(len(sec) + len(rootNoSubdir) + 8)
	root := buildDentry(attrDirectory, childOffset, "", "", nil)

	meta := append(append(append(sec, root...), endMarker()...), childList...)
	return meta, childOffset
}

func TestSyntheticMetadataTree(t *testing.T) {
	meta, _ := buildSyntheticMetadata()
	root, err := parseImageMetadata(meta)
	if err != nil {
		t.Fatalf("parseImageMetadata: %v", err)
	}
	if !root.IsDir() {
		t.Fatal("root not a directory")
	}
	children := root.Children()
	if len(children) != 2 {
		t.Fatalf("root has %d children, want 2", len(children))
	}
	if children[0].Name != "file.txt" || children[0].ShortName != "FILE~1.TXT" {
		t.Errorf("child0 = %+v", children[0])
	}
	if children[1].Name != "data.bin" || children[1].IsDir() {
		t.Errorf("child1 = %+v", children[1])
	}
	if children[0].Children() != nil {
		t.Error("a file should have no children")
	}
}

// TestOpenImageUncompressedMetadata covers the uncompressed metadata-resource
// path via a WIM whose single image metadata is stored uncompressed.
func TestOpenImageUncompressedMetadata(t *testing.T) {
	meta, _ := buildSyntheticMetadata()
	le := binary.LittleEndian

	const tableOffset = headerSize
	tableSize := blobTableEntrySize
	metaOffset := tableOffset + tableSize

	hdr := make([]byte, headerSize)
	copy(hdr, imageTag[:])
	le.PutUint32(hdr[44:], 1)
	le.PutUint64(hdr[48:], uint64(tableSize)) // OffsetTable, uncompressed
	le.PutUint64(hdr[56:], tableOffset)
	le.PutUint64(hdr[64:], uint64(tableSize))

	entry := make([]byte, blobTableEntrySize)
	// metadata flag only (no compressed flag) -> read raw.
	le.PutUint64(entry[0:], uint64(resFlagMetadata)<<56|uint64(len(meta)))
	le.PutUint64(entry[8:], uint64(metaOffset))
	le.PutUint64(entry[16:], uint64(len(meta)))

	data := append(append(hdr, entry...), meta...)

	w, err := OpenReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("OpenReaderAt: %v", err)
	}
	root, err := w.OpenImage(1)
	if err != nil {
		t.Fatalf("OpenImage: %v", err)
	}
	if len(root.Children()) != 2 {
		t.Fatalf("got %d children, want 2", len(root.Children()))
	}
}
