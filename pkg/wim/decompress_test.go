package wim

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

// TestChunkedReaderPassthrough exercises the chunk-table parsing and multi-chunk
// reassembly with chunks stored uncompressed (compressed size == chunk size), so
// no decompressor is needed to validate the framing.
func TestChunkedReaderPassthrough(t *testing.T) {
	const chunkSize = int64(8)
	c0 := []byte("AAAAAAAA")
	c1 := []byte("BBBBBBBB")
	c2 := []byte("CCCC") // final, short chunk
	original := bytes.Join([][]byte{c0, c1, c2}, nil)

	// 3 chunks -> chunk table has 2 entries (offsets of chunks 1 and 2),
	// 4 bytes each, relative to the end of the table.
	table := make([]byte, 8)
	binary.LittleEndian.PutUint32(table[0:], 8)  // chunk 1 starts 8 bytes in
	binary.LittleEndian.PutUint32(table[4:], 16) // chunk 2 starts 16 bytes in
	resource := bytes.Join([][]byte{table, c0, c1, c2}, nil)

	cr, err := newChunkedReader(bytes.NewReader(resource), CompressionLZX,
		chunkSize, int64(len(resource)), int64(len(original)))
	if err != nil {
		t.Fatalf("newChunkedReader: %v", err)
	}
	defer cr.Close()

	got, err := io.ReadAll(cr)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("got %q, want %q", got, original)
	}
}

// TestChunkedReaderXPRESS reads a single-chunk XPRESS resource through the
// chunked reader, validating the dispatch wiring. The chunk is the same
// hand-built "ABC" vector used in the xpress package tests.
func TestChunkedReaderXPRESS(t *testing.T) {
	lens := make([]byte, 256)
	lens[32] = 1 << 4       // symbol 'A' (65): length 1
	lens[33] = 2 | (2 << 4) // symbols 'B' (66) and 'C' (67): length 2
	chunk := append(lens, 0x00, 0x58)

	// Single chunk: no chunk table precedes the data.
	cr, err := newChunkedReader(bytes.NewReader(chunk), CompressionXPRESS, 32768, int64(len(chunk)), 3)
	if err != nil {
		t.Fatalf("newChunkedReader: %v", err)
	}
	defer cr.Close()
	got, err := io.ReadAll(cr)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "ABC" {
		t.Fatalf("got %q, want ABC", got)
	}
}

func TestChunkedReaderUnsupported(t *testing.T) {
	// An unknown compression algorithm is reported as unsupported.
	const chunkSize = int64(8)
	resource := make([]byte, 4) // one chunk, compressed (compSize 4 != uncomp 8)
	_, err := newChunkedReader(bytes.NewReader(resource), Compression(99), chunkSize, 4, 8)
	if !errors.Is(err, errCompressionUnsupported) {
		t.Fatalf("got %v, want errCompressionUnsupported", err)
	}
}

// TestResourceReaderUncompressed reads an uncompressed resource (the XML
// catalog) back through resourceReader.
func TestResourceReaderUncompressed(t *testing.T) {
	data := buildWIM(0, 32768, 1, `<WIM><IMAGE INDEX="1"><NAME>X</NAME></IMAGE></WIM>`)
	w, err := OpenReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	rc, err := w.resourceReader(w.hdr.XMLData)
	if err != nil {
		t.Fatalf("resourceReader: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != int(w.hdr.XMLData.OriginalSize) {
		t.Fatalf("read %d bytes, want %d", len(got), w.hdr.XMLData.OriginalSize)
	}

	// A solid resource is not yet supported by this path.
	solidRD := resourceDescriptor{Flags: resFlagCompressed | resFlagSpanned, CompressedSize: 10, OriginalSize: 20}
	if _, err := w.resourceReader(solidRD); !errors.Is(err, errCompressionUnsupported) {
		t.Fatalf("solid resource: got %v, want errCompressionUnsupported", err)
	}
}
