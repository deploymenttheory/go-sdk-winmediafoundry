package wim

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func metaWith(dentry []byte) []byte {
	meta := make([]byte, 8) // empty security header
	return append(meta, dentry...)
}

func TestDentryParseErrors(t *testing.T) {
	le := binary.LittleEndian

	t.Run("dentry too short", func(t *testing.T) {
		d := make([]byte, 16)
		le.PutUint64(d, 10) // length 10 < direntrySize
		if _, err := parseImageMetadata(metaWith(d)); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("dentry past buffer", func(t *testing.T) {
		d := make([]byte, 16)
		le.PutUint64(d, 1000) // claims 1000 bytes, only 16 present
		if _, err := parseImageMetadata(metaWith(d)); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("names exceed length", func(t *testing.T) {
		d := buildDentry(0x20, 0, "abcdef", "", nil)
		le.PutUint64(d[0:], 104)    // shrink the length below the names region
		le.PutUint16(d[100:], 0xff) // huge file name length
		if _, err := parseImageMetadata(metaWith(d)); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("truncated stream", func(t *testing.T) {
		// A dentry that declares a stream but supplies no stream bytes.
		d := buildDentry(0x20, 0, "x", "", nil)
		le.PutUint16(d[96:], 1) // streamCount = 1, but no stream follows
		if _, err := parseImageMetadata(metaWith(d)); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("security table too big", func(t *testing.T) {
		meta := make([]byte, 16)
		le.PutUint32(meta, 0xffff) // TotalLength larger than the buffer
		if _, err := parseImageMetadata(meta); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestOpenImageOffsetTableError(t *testing.T) {
	le := binary.LittleEndian
	hdr := make([]byte, headerSize)
	copy(hdr, imageTag[:])
	le.PutUint32(hdr[44:], 1)
	// OffsetTable points far beyond the file.
	le.PutUint64(hdr[48:], 100)
	le.PutUint64(hdr[56:], 1<<40)
	le.PutUint64(hdr[64:], 100)

	w, err := OpenReaderAt(bytes.NewReader(hdr), int64(len(hdr)))
	if err != nil {
		t.Fatalf("OpenReaderAt: %v", err)
	}
	if _, err := w.OpenImage(1); err == nil {
		t.Fatal("expected error reading offset table beyond EOF")
	}
}
