package wim

import (
	"bytes"
	"crypto/sha1" //nolint:gosec // content-addressed blob hash, not security
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// buildUncompressedSolid builds a solid resource whose chunks are stored
// uncompressed (compression_format 0), which exercises the solid framing
// (alt header, chunk table, chunk-spanning reads) without needing a compressor.
func buildUncompressedSolid(content []byte, chunkSize int) []byte {
	le := binary.LittleEndian
	n := len(content)
	numChunks := (n + chunkSize - 1) / chunkSize

	hdr := make([]byte, 16)
	le.PutUint64(hdr[0:], uint64(n))
	le.PutUint32(hdr[8:], uint32(chunkSize))
	le.PutUint32(hdr[12:], 0) // none
	table := make([]byte, numChunks*4)
	for i := range numChunks {
		cs := chunkSize
		if i == numChunks-1 {
			cs = n - i*chunkSize
		}
		le.PutUint32(table[i*4:], uint32(cs))
	}
	return append(append(hdr, table...), content...)
}

func TestSolidResourceReadAt(t *testing.T) {
	content := []byte("ABCDEFGHIJKLMNOPQRST") // 20 bytes
	solid := buildUncompressedSolid(content, 8) // chunks of 8, 8, 4
	w := &WIM{r: bytes.NewReader(solid)}

	s, err := w.newSolidResource(0, int64(len(solid)))
	if err != nil {
		t.Fatalf("newSolidResource: %v", err)
	}
	if s.numChunks() != 3 {
		t.Fatalf("numChunks = %d, want 3", s.numChunks())
	}

	cases := []struct{ off, size int64 }{{0, 20}, {5, 10}, {16, 4}, {8, 8}, {0, 1}}
	for _, c := range cases {
		got, err := s.readAt(c.off, c.size)
		if err != nil {
			t.Fatalf("readAt(%d,%d): %v", c.off, c.size, err)
		}
		if want := content[c.off : c.off+c.size]; !bytes.Equal(got, want) {
			t.Errorf("readAt(%d,%d) = %q, want %q", c.off, c.size, got, want)
		}
	}

	if _, err := s.readAt(0, 21); err == nil {
		t.Error("expected out-of-range error")
	}
}

const (
	metaFixtureOrigSize = 206128
	metaFixtureSHA1     = "c6d0872da60175857d807d5714f49240b6adcff9"
)

func readMetaFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "lzms-metadata-resource.bin"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

// TestSolidLZMSChunk repackages the real LZMS fixture (a non-solid chunked
// resource: a 4-byte chunk table followed by two 128 KiB LZMS chunks) into the
// solid (alt chunk-table) format, exercising decompressChunk's LZMS path across
// two chunks.
func TestSolidLZMSChunk(t *testing.T) {
	fixture := readMetaFixture(t)
	le := binary.LittleEndian

	// The non-solid fixture begins with one 4-byte entry: the compressed size of
	// chunk 0 (offset of chunk 1). Two chunks total for 206128 bytes @ 128 KiB.
	chunk0Len := int(le.Uint32(fixture[0:4]))
	chunk0 := fixture[4 : 4+chunk0Len]
	chunk1 := fixture[4+chunk0Len:]

	hdr := make([]byte, 16)
	le.PutUint64(hdr[0:], metaFixtureOrigSize)
	le.PutUint32(hdr[8:], 131072) // chunk size
	le.PutUint32(hdr[12:], 3)     // LZMS
	table := make([]byte, 8)      // two chunks -> two compressed sizes
	le.PutUint32(table[0:], uint32(len(chunk0)))
	le.PutUint32(table[4:], uint32(len(chunk1)))
	solid := bytes.Join([][]byte{hdr, table, chunk0, chunk1}, nil)

	w := &WIM{r: bytes.NewReader(solid)}
	s, err := w.newSolidResource(0, int64(len(solid)))
	if err != nil {
		t.Fatalf("newSolidResource: %v", err)
	}
	if s.numChunks() != 2 {
		t.Fatalf("numChunks = %d, want 2", s.numChunks())
	}
	got, err := s.readAt(0, metaFixtureOrigSize)
	if err != nil {
		t.Fatalf("readAt: %v", err)
	}
	sum := sha1.Sum(got) //nolint:gosec
	if hex.EncodeToString(sum[:]) != metaFixtureSHA1 {
		t.Fatalf("solid LZMS chunk SHA-1 mismatch")
	}
}

// TestReadFileCompressedBlob covers the standalone (non-solid) compressed blob
// path of blobBytes, using the LZMS fixture as a regular compressed resource.
func TestReadFileCompressedBlob(t *testing.T) {
	fixture := readMetaFixture(t)
	var blobHash [20]byte
	h, _ := hex.DecodeString(metaFixtureSHA1)
	copy(blobHash[:], h)

	// Metadata: root with one file referencing the compressed blob.
	child := buildDentry(0x20, 0, "big.bin", "", nil)
	copy(child[64:84], blobHash[:])
	rootNoSub := buildDentry(attrDirectory, 0, "", "", nil)
	childOffset := int64(8 + len(rootNoSub) + 8)
	root := buildDentry(attrDirectory, childOffset, "", "", nil)
	meta := bytes.Join([][]byte{make([]byte, 8), root, endMarker(), child, endMarker()}, nil)

	le := binary.LittleEndian
	const tableOffset = headerSize
	tableSize := 2 * blobTableEntrySize
	metaOffset := tableOffset + tableSize
	blobOffset := metaOffset + len(meta)

	hdr := make([]byte, headerSize)
	copy(hdr, imageTag[:])
	le.PutUint32(hdr[16:], flagCompressLZMS|flagCompressed)
	le.PutUint32(hdr[20:], 131072) // chunk size
	le.PutUint32(hdr[44:], 1)
	le.PutUint64(hdr[48:], uint64(tableSize))
	le.PutUint64(hdr[56:], tableOffset)
	le.PutUint64(hdr[64:], uint64(tableSize))

	mkEntry := func(flags byte, compSize, offset, origSize int64, hh []byte) []byte {
		e := make([]byte, blobTableEntrySize)
		le.PutUint64(e[0:], uint64(flags)<<56|uint64(compSize))
		le.PutUint64(e[8:], uint64(offset))
		le.PutUint64(e[16:], uint64(origSize))
		copy(e[30:], hh)
		return e
	}
	table := bytes.Join([][]byte{
		mkEntry(resFlagMetadata, int64(len(meta)), int64(metaOffset), int64(len(meta)), nil),
		mkEntry(resFlagCompressed, int64(len(fixture)), int64(blobOffset), metaFixtureOrigSize, blobHash[:]),
	}, nil)
	data := bytes.Join([][]byte{hdr, table, meta, fixture}, nil)

	w, err := OpenReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("OpenReaderAt: %v", err)
	}
	root2, err := w.OpenImage(1)
	if err != nil {
		t.Fatalf("OpenImage: %v", err)
	}
	got, err := w.ReadFile(root2.Children()[0])
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	sum := sha1.Sum(got) //nolint:gosec
	if hex.EncodeToString(sum[:]) != metaFixtureSHA1 {
		t.Fatalf("compressed blob SHA-1 mismatch")
	}
}

func TestCompressionFromFormat(t *testing.T) {
	for f, want := range map[uint32]Compression{
		0: CompressionNone, 1: CompressionXPRESS, 2: CompressionLZX,
		3: CompressionLZMS, 99: CompressionNone,
	} {
		if got := compressionFromFormat(f); got != want {
			t.Errorf("compressionFromFormat(%d) = %v, want %v", f, got, want)
		}
	}
}

func TestSolidUnsupportedCompression(t *testing.T) {
	// A solid resource declaring XPRESS, which the solid reader does not support.
	le := binary.LittleEndian
	hdr := make([]byte, 16)
	le.PutUint64(hdr[0:], 8)  // res_usize
	le.PutUint32(hdr[8:], 8)  // chunk_size
	le.PutUint32(hdr[12:], 1) // XPRESS
	table := make([]byte, 4)
	le.PutUint32(table, 8)
	solid := append(append(hdr, table...), make([]byte, 8)...)

	w := &WIM{r: bytes.NewReader(solid)}
	s, err := w.newSolidResource(0, int64(len(solid)))
	if err != nil {
		t.Fatalf("newSolidResource: %v", err)
	}
	if _, err := s.readAt(0, 8); err == nil {
		t.Fatal("expected unsupported-compression error")
	}
}

// TestExtractMixed covers regular (non-solid) blobs, empty files, and reparse
// points alongside a solid blob.
func TestExtractMixed(t *testing.T) {
	c1 := []byte("content of the solid file")
	c2 := []byte("content of the regular file")
	h1, h2 := sha1.Sum(c1), sha1.Sum(c2) //nolint:gosec

	solid := buildUncompressedSolid(c1, 64)

	solidFile := buildDentry(0x20, 0, "solid.bin", "", nil)
	copy(solidFile[64:84], h1[:])
	regularFile := buildDentry(0x20, 0, "regular.txt", "", nil)
	copy(regularFile[64:84], h2[:])
	emptyFile := buildDentry(0x20, 0, "empty.txt", "", nil) // zero hash
	reparseFile := buildDentry(attrReparsePoint|0x20, 0, "link", "", nil)

	rootNoSub := buildDentry(attrDirectory, 0, "", "", nil)
	// An empty subdirectory whose entries point at the root end marker (length 0).
	subDir := buildDentry(attrDirectory, int64(8+len(rootNoSub)), "sub", "", nil)

	childList := bytes.Join([][]byte{solidFile, regularFile, emptyFile, reparseFile, subDir, endMarker()}, nil)
	childOffset := int64(8 + len(rootNoSub) + 8)
	root := buildDentry(attrDirectory, childOffset, "", "", nil)
	meta := bytes.Join([][]byte{make([]byte, 8), root, endMarker(), childList}, nil)

	le := binary.LittleEndian
	const tableOffset = headerSize
	tableSize := 4 * blobTableEntrySize
	metaOffset := tableOffset + tableSize
	regularOffset := metaOffset + len(meta)
	solidOffset := regularOffset + len(c2)

	hdr := make([]byte, headerSize)
	copy(hdr, imageTag[:])
	le.PutUint32(hdr[44:], 1)
	le.PutUint64(hdr[48:], uint64(tableSize))
	le.PutUint64(hdr[56:], tableOffset)
	le.PutUint64(hdr[64:], uint64(tableSize))

	mkEntry := func(flags byte, compSize, offset, origSize int64, h []byte) []byte {
		e := make([]byte, blobTableEntrySize)
		le.PutUint64(e[0:], uint64(flags)<<56|uint64(compSize))
		le.PutUint64(e[8:], uint64(offset))
		le.PutUint64(e[16:], uint64(origSize))
		copy(e[30:], h)
		return e
	}
	table := bytes.Join([][]byte{
		mkEntry(resFlagMetadata, int64(len(meta)), int64(metaOffset), int64(len(meta)), nil),
		mkEntry(0, int64(len(c2)), int64(regularOffset), int64(len(c2)), h2[:]),
		mkEntry(resFlagSolid, int64(len(solid)), int64(solidOffset), solidResourceMagic, nil),
		mkEntry(resFlagSolid, int64(len(c1)), 0, 0, h1[:]),
	}, nil)

	data := bytes.Join([][]byte{hdr, table, meta, c2, solid}, nil)

	w, err := OpenReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("OpenReaderAt: %v", err)
	}
	dir := t.TempDir()
	if err := w.ExtractImage(1, dir); err != nil {
		t.Fatalf("ExtractImage: %v", err)
	}

	check := func(name string, want []byte) {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s = %q (%v), want %q", name, got, err, want)
		}
	}
	check("solid.bin", c1)
	check("regular.txt", c2)
	check("empty.txt", []byte{})
	if _, err := os.Stat(filepath.Join(dir, "link")); err == nil {
		t.Error("reparse point should not be extracted")
	}
	if st, err := os.Stat(filepath.Join(dir, "sub")); err != nil || !st.IsDir() {
		t.Errorf("subdirectory not extracted: %v", err)
	}

	// Error paths.
	if _, err := w.ReadFile(&File{Name: "ghost", Attributes: 0x20, Hash: [20]byte{0xde, 0xad}}); err == nil {
		t.Error("ReadFile of an unknown blob should error")
	}
	if err := w.ExtractImage(99, dir); err == nil {
		t.Error("ExtractImage with a bad index should error")
	}
}

func TestOpenViaPath(t *testing.T) {
	data := buildWIMWithMetadata(t)
	path := filepath.Join(t.TempDir(), "test.wim")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	w, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer w.Close()
	if w.ImageCount() != 1 {
		t.Errorf("ImageCount = %d, want 1", w.ImageCount())
	}
	if _, err := Open(filepath.Join(t.TempDir(), "missing.wim")); err == nil {
		t.Error("Open of a missing file should error")
	}
}

// TestExtractImageFromSolid builds a complete WIM whose single file's content
// lives in an (uncompressed) solid resource, then extracts the image and checks
// the file content — exercising the offset-table solid run, blob mapping, the
// dentry tree, and extraction end to end.
func TestExtractImageFromSolid(t *testing.T) {
	content := []byte("the quick brown fox jumps over the lazy dog\n")
	hash := sha1.Sum(content) //nolint:gosec

	solid := buildUncompressedSolid(content, 64) // single chunk

	// Metadata: root directory containing one file whose hash points at the blob.
	child := buildDentry(0x20, 0, "readme.txt", "", nil)
	copy(child[64:84], hash[:]) // set the file's content hash
	rootNoSub := buildDentry(attrDirectory, 0, "", "", nil)
	childOffset := int64(8 + len(rootNoSub) + 8)
	root := buildDentry(attrDirectory, childOffset, "", "", nil)
	meta := bytes.Join([][]byte{make([]byte, 8), root, endMarker(), child, endMarker()}, nil)

	le := binary.LittleEndian
	const tableOffset = headerSize
	tableSize := 3 * blobTableEntrySize
	metaOffset := tableOffset + tableSize
	solidOffset := metaOffset + len(meta)

	hdr := make([]byte, headerSize)
	copy(hdr, imageTag[:])
	le.PutUint32(hdr[12:], 0x00000e00)
	le.PutUint32(hdr[44:], 1) // image count
	le.PutUint64(hdr[48:], uint64(tableSize))
	le.PutUint64(hdr[56:], tableOffset)
	le.PutUint64(hdr[64:], uint64(tableSize))

	mkEntry := func(flags byte, compSize, offset, origSize int64, h []byte) []byte {
		e := make([]byte, blobTableEntrySize)
		le.PutUint64(e[0:], uint64(flags)<<56|uint64(compSize))
		le.PutUint64(e[8:], uint64(offset))
		le.PutUint64(e[16:], uint64(origSize))
		copy(e[30:], h)
		return e
	}
	var table []byte
	table = append(table, mkEntry(resFlagMetadata, int64(len(meta)), int64(metaOffset), int64(len(meta)), nil)...)
	table = append(table, mkEntry(resFlagSolid, int64(len(solid)), int64(solidOffset), solidResourceMagic, nil)...)
	table = append(table, mkEntry(resFlagSolid, int64(len(content)), 0, 0, hash[:])...)

	data := bytes.Join([][]byte{hdr, table, meta, solid}, nil)

	w, err := OpenReaderAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("OpenReaderAt: %v", err)
	}

	dir := t.TempDir()
	if err := w.ExtractImage(1, dir); err != nil {
		t.Fatalf("ExtractImage: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "readme.txt"))
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("extracted content = %q, want %q", got, content)
	}

	// ReadFile directly, and the directory/empty-file paths.
	root2, _ := w.OpenImage(1)
	for _, c := range root2.Children() {
		if c.Name == "readme.txt" {
			b, err := w.ReadFile(c)
			if err != nil || !bytes.Equal(b, content) {
				t.Errorf("ReadFile = %q, %v", b, err)
			}
		}
	}
	if _, err := w.ReadFile(root2); err == nil {
		t.Error("ReadFile on a directory should error")
	}
}
