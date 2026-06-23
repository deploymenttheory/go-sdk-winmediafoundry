package wim

import "fmt"

// blobTableEntrySize is the on-disk size of one offset-table entry:
// a 24-byte resource header, 2-byte part number, 4-byte refcount, 20-byte SHA-1.
const blobTableEntrySize = 50

// loadOffsetTable parses the (uncompressed) offset/blob table once: it records
// the per-image metadata resources in image order and indexes regular blobs by
// hash. Solid (packed) resources are skipped here; reading file contents out of
// them is handled separately.
func (w *WIM) loadOffsetTable() error {
	if w.blobTableLoaded {
		return nil
	}
	raw, err := w.readResourceRaw(w.hdr.OffsetTable)
	if err != nil {
		return fmt.Errorf("wim: read offset table: %w", err)
	}

	w.blobs = make(map[[20]byte]blobLocation)
	n := len(raw) / blobTableEntrySize
	for i := 0; i < n; {
		rd, hash := parseBlobEntry(raw, i)
		switch {
		case rd.Flags&resFlagMetadata != 0:
			w.metadataRes = append(w.metadataRes, rd)
			i++
		case rd.Flags&resFlagSolid != 0:
			next, err := w.loadSolidRun(raw, i, n)
			if err != nil {
				return err
			}
			i = next
		default:
			if !isZeroHash(hash) {
				w.blobs[hash] = blobLocation{rd: rd, size: rd.OriginalSize}
			}
			i++
		}
	}
	w.blobTableLoaded = true
	return nil
}

func parseBlobEntry(raw []byte, i int) (resourceDescriptor, [20]byte) {
	entry := raw[i*blobTableEntrySize : (i+1)*blobTableEntrySize]
	rd := parseResource(entry[0:24])
	var hash [20]byte
	copy(hash[:], entry[30:50])
	return rd, hash
}

func isZeroHash(h [20]byte) bool { return h == [20]byte{} }

// loadSolidRun processes a contiguous run of solid (packed) offset-table entries
// starting at index start: it builds the run's solid resources, then assigns
// each blob in the run to the resource and offset that hold its uncompressed
// bytes. It returns the index just past the run.
func (w *WIM) loadSolidRun(raw []byte, start, n int) (int, error) {
	// First pass: the run extends while entries are solid; collect the solid
	// resource descriptors (entries whose uncompressed_size is the magic value).
	end := start
	var resources []*solidResource
	for end < n {
		rd, _ := parseBlobEntry(raw, end)
		if rd.Flags&resFlagSolid == 0 {
			break
		}
		if rd.OriginalSize == solidResourceMagic {
			res, err := w.newSolidResource(rd.Offset, rd.CompressedSize)
			if err != nil {
				return 0, err
			}
			resources = append(resources, res)
		}
		end++
	}

	// Second pass: assign blob entries to a solid resource by uncompressed
	// offset (offsets span the run's resources in order).
	for i := start; i < end; i++ {
		rd, hash := parseBlobEntry(raw, i)
		if rd.OriginalSize == solidResourceMagic || isZeroHash(hash) {
			continue
		}
		offset, size := rd.Offset, rd.CompressedSize // uncompressed offset and size
		for _, res := range resources {
			if offset+size <= res.uncompSize {
				w.blobs[hash] = blobLocation{solid: res, offset: offset, size: size}
				break
			}
			offset -= res.uncompSize
		}
	}
	return end, nil
}

// ImageCount returns the number of images in the WIM.
func (w *WIM) ImageCount() int { return int(w.hdr.ImageCount) }

// OpenImage parses the directory tree of the 1-based image index and returns its
// root directory. The image's metadata is decompressed on demand.
func (w *WIM) OpenImage(index int) (*File, error) {
	if err := w.loadOffsetTable(); err != nil {
		return nil, err
	}
	if index < 1 || index > len(w.metadataRes) {
		return nil, fmt.Errorf("wim: image index %d out of range (1..%d)", index, len(w.metadataRes))
	}
	meta, err := w.readMetadataResource(w.metadataRes[index-1])
	if err != nil {
		return nil, fmt.Errorf("wim: read image %d metadata: %w", index, err)
	}
	root, err := parseImageMetadata(meta)
	if err != nil {
		return nil, fmt.Errorf("wim: parse image %d metadata: %w", index, err)
	}
	return root, nil
}
