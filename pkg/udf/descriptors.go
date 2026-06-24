package udf

import "encoding/binary"

const (
	fileTypeDirectory = 4
	fileTypeRegular   = 5

	// filePermsReadAll grants read+execute to owner/group/other.
	filePermsReadAll = 0x14A5

	// maxExtentLen is the largest byte length a single short allocation
	// descriptor may record. A short_ad's length field is 30 bits — the top two
	// bits are the extent type — so it cannot reach 0x40000000 (1 GiB), and the
	// length of every non-final extent must be a multiple of the logical block
	// size. 0x3FFFF800 (== 524287*2048) is the greatest block-aligned value below
	// that limit. Files larger than this (e.g. boot.wim ~1.5 GiB, install.wim
	// >4 GiB) must be split across several extents; a single short_ad would
	// overflow its length field into the extent-type bits and produce media that
	// lenient readers tolerate but the Windows boot manager cannot read.
	maxExtentLen = 0x3FFFF800
)

// volStructDesc builds a Volume Structure Descriptor (BEA01/NSR02/TEA01) for the
// Volume Recognition Sequence. These are not tagged descriptors.
func volStructDesc(id string) []byte {
	b := make([]byte, SectorSize)
	b[0] = 0 // structure type
	copy(b[1:6], id)
	b[6] = 1 // structure version
	return b
}

// putAnchor writes an Anchor Volume Descriptor Pointer pointing at the main and
// reserve volume descriptor sequences.
func putAnchor(b []byte, location uint32) {
	copy(b[16:], extentAD(vdsSectors*SectorSize, lbnMainVDS))
	copy(b[24:], extentAD(vdsSectors*SectorSize, lbnReserveVDS))
	putTag(b[:512], tagAnchorVolumePointer, location)
}

// extentAD writes an 8-byte extent allocation descriptor (length, location).
func extentAD(length, location uint32) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint32(b[0:], length)
	binary.LittleEndian.PutUint32(b[4:], location)
	return b
}

func (w *imageWriter) primaryVolumeDescriptor(loc uint32) []byte {
	b := make([]byte, SectorSize)
	le := binary.LittleEndian
	le.PutUint32(b[16:], 1) // volume descriptor sequence number
	le.PutUint32(b[20:], 0) // primary volume descriptor number
	copy(b[24:], encodeDString(w.volumeID, 32))
	le.PutUint16(b[56:], 1)                      // volume sequence number
	le.PutUint16(b[58:], 1)                      // max volume sequence number
	le.PutUint16(b[60:], 2)                      // interchange level
	le.PutUint16(b[62:], 2)                      // max interchange level
	le.PutUint32(b[64:], 1)                      // character set list
	le.PutUint32(b[68:], 1)                      // max character set list
	copy(b[72:], encodeDString(w.volumeID, 128)) // volume set identifier
	copy(b[200:], charSpec())                    // descriptor character set
	copy(b[264:], charSpec())                    // explanatory character set
	copy(b[376:], encodeTimestamp(w.now))
	copy(b[388:], implEntityID())
	putTag(b[:490], tagPrimaryVolume, loc)
	return b
}

func (w *imageWriter) implUseVolumeDescriptor(loc uint32) []byte {
	b := make([]byte, SectorSize)
	le := binary.LittleEndian
	le.PutUint32(b[16:], 2) // volume descriptor sequence number
	// Implementation identifier: the UDF LV Info entity.
	suffix := make([]byte, 8)
	le.PutUint16(suffix[0:], 0x0102) // UDF revision
	copy(b[20:], entityID("*UDF LV Info", suffix))
	// Implementation use: LVI charsets + identifier + impl-use entity.
	copy(b[52:], charSpec())
	copy(b[116:], encodeDString(w.volumeID, 128)) // logical volume identifier
	copy(b[116+128:], encodeDString("", 36))      // LVInfo1..3 (left blank)
	copy(b[116+128+36*3:], implEntityID())
	putTag(b[:512], tagImplementationUseVol, loc)
	return b
}

func (w *imageWriter) partitionDescriptor(loc, partitionBlocks uint32) []byte {
	b := make([]byte, SectorSize)
	le := binary.LittleEndian
	le.PutUint32(b[16:], 3) // volume descriptor sequence number
	le.PutUint16(b[20:], 1) // partition flags: allocated
	le.PutUint16(b[22:], 0) // partition number
	copy(b[24:], entityID("+NSR02", nil))
	le.PutUint32(b[184:], 1)              // access type: read-only
	le.PutUint32(b[188:], lbnPartitionLB) // partition starting location (absolute)
	le.PutUint32(b[192:], partitionBlocks)
	copy(b[196:], implEntityID())
	putTag(b[:356], tagPartition, loc)
	return b
}

func (w *imageWriter) logicalVolumeDescriptor(loc uint32) []byte {
	b := make([]byte, SectorSize)
	le := binary.LittleEndian
	le.PutUint32(b[16:], 4) // volume descriptor sequence number
	copy(b[20:], charSpec())
	copy(b[84:], encodeDString(w.volumeID, 128))
	le.PutUint32(b[212:], SectorSize) // logical block size
	copy(b[216:], domainEntityID())
	// LogicalVolumeContentsUse: long_ad to the File Set Descriptor at partition LB 0.
	copy(b[248:], longAD(SectorSize, 0, 0))
	le.PutUint32(b[264:], 6) // map table length
	le.PutUint32(b[268:], 1) // number of partition maps
	copy(b[272:], implEntityID())
	copy(b[432:], extentAD(2*SectorSize, lbnIntegrity)) // integrity sequence extent
	// Partition map (type 1): type, length, volume seq number, partition number.
	b[440] = 1
	b[441] = 6
	le.PutUint16(b[442:], 1) // volume sequence number
	le.PutUint16(b[444:], 0) // partition number
	putTag(b[:446], tagLogicalVolume, loc)
	return b
}

func (w *imageWriter) unallocatedSpaceDescriptor(loc uint32) []byte {
	b := make([]byte, SectorSize)
	binary.LittleEndian.PutUint32(b[16:], 5) // volume descriptor sequence number
	binary.LittleEndian.PutUint32(b[20:], 0) // number of allocation descriptors
	putTag(b[:24], tagUnallocatedSpace, loc)
	return b
}

func terminatingDescriptor(loc uint32) []byte {
	b := make([]byte, SectorSize)
	putTag(b[:16], tagTerminating, loc)
	return b
}

func (w *imageWriter) integrityDescriptor(partitionBlocks uint32) []byte {
	b := make([]byte, SectorSize)
	le := binary.LittleEndian
	copy(b[16:], encodeTimestamp(w.now))
	le.PutUint32(b[28:], 1) // integrity type: close
	// NextIntegrityExtent (8) @32 = 0.
	// LogicalVolumeContentsUse (32) @40: next unique id at [0:8].
	le.PutUint64(b[40:], 16)              // next unique id (above those we assigned)
	le.PutUint32(b[72:], 1)               // number of partitions
	le.PutUint32(b[76:], 0)               // length of implementation use
	le.PutUint32(b[80:], 0xFFFFFFFF)      // free space table: unknown
	le.PutUint32(b[84:], partitionBlocks) // size table
	putTag(b[:88], tagLogicalVolumeInteg, lbnIntegrity)
	return b
}

// fileEntry builds a File Entry descriptor for n.
func (w *imageWriter) fileEntry(n *node, fileType uint8) []byte {
	b := make([]byte, SectorSize)
	le := binary.LittleEndian

	// ICB tag (20 bytes at offset 16).
	le.PutUint16(b[16+4:], 4)  // strategy type
	le.PutUint16(b[16+8:], 1)  // maximum number of entries
	b[16+11] = fileType        // file type
	le.PutUint16(b[16+18:], 0) // flags: short allocation descriptors

	le.PutUint32(b[36:], 0xFFFFFFFF) // uid: unset
	le.PutUint32(b[40:], 0xFFFFFFFF) // gid: unset
	le.PutUint32(b[44:], filePermsReadAll)
	linkCount := uint16(1)
	if n.isDir {
		linkCount = 2 + uint16(countChildDirs(n))
	}
	le.PutUint16(b[48:], linkCount)

	// information length: a directory's data is its FID list (always < 4 GiB),
	// a regular file's is its true byte size (which may exceed 4 GiB, so it must
	// not be taken from the uint32 n.dataLen).
	infoLen := uint64(n.dataLen)
	if fileType == fileTypeRegular {
		infoLen = uint64(n.size)
	}
	le.PutUint64(b[56:], infoLen)                 // information length
	le.PutUint64(b[64:], uint64(blocks(infoLen))) // logical blocks recorded
	copy(b[72:], encodeTimestamp(n.modTime))
	copy(b[84:], encodeTimestamp(n.modTime))
	copy(b[96:], encodeTimestamp(n.modTime))
	le.PutUint32(b[108:], 1) // checkpoint
	copy(b[128:], implEntityID())
	le.PutUint64(b[160:], n.uniqueID)
	le.PutUint32(b[168:], 0) // length of extended attributes

	// Allocation descriptors: the data occupies a contiguous run of logical
	// blocks starting at n.dataLB, but a single short_ad cannot describe more
	// than maxExtentLen bytes, so split the run into successive block-aligned
	// extents (the final one carries the exact remaining byte count). All extents
	// are type 0 (recorded and allocated): maxExtentLen keeps the top two bits of
	// each length field clear. The descriptors fit comfortably in the File Entry
	// (a >4 GiB install.wim needs only a handful).
	contentLen := 176
	if infoLen > 0 {
		adOff := 176
		block := n.dataLB
		remaining := infoLen
		for remaining > 0 {
			ext := min(remaining, maxExtentLen)
			copy(b[adOff:], shortAD(uint32(ext), block))
			block += blocks(ext)
			adOff += 8
			remaining -= ext
		}
		le.PutUint32(b[172:], uint32(adOff-176)) // length of allocation descriptors
		contentLen = adOff
	}
	putTag(b[:contentLen], tagFileEntry, n.feBlock)
	return b
}

func countChildDirs(n *node) int {
	count := 0
	for _, c := range n.children {
		if c.isDir {
			count++
		}
	}
	return count
}

// --- File Identifier Descriptors ---

// dirFIDBytes returns the (block-boundary-padded) byte size of a directory's FID
// list: a parent entry plus one entry per child.
func dirFIDBytes(n *node) uint32 {
	lens := []int{fidLen("")}
	for _, c := range n.children {
		lens = append(lens, fidLen(c.name))
	}
	off := 0
	for _, l := range lens {
		off = fidAdvance(off, l)
	}
	return uint32(off)
}

// fidAdvance returns the offset after placing a FID of length l at off, padding
// to the next block first if the FID would cross a logical-block boundary (FIDs
// may not span blocks).
func fidAdvance(off, l int) int {
	if off/SectorSize != (off+l-1)/SectorSize {
		off = (off/SectorSize + 1) * SectorSize
	}
	return off + l
}

func fidLen(name string) int {
	base := 38 + dcharsLen(name)
	return (base + 3) / 4 * 4
}

func dcharsLen(name string) int {
	if name == "" {
		return 0
	}
	eightBit := true
	n := 0
	for _, r := range name {
		if r > 0xFF {
			eightBit = false
		}
		n++
	}
	if eightBit {
		return 1 + n
	}
	return 1 + 2*utf16Count(name)
}

// appendFID appends one File Identifier Descriptor to buf (padding to a block
// boundary first if needed), referencing childFE. baseLB is the first logical
// block of the FID extent (used to compute the per-FID tag location).
func appendFID(buf []byte, baseLB uint32, name string, childFE uint32, isDir bool) []byte {
	dchars := encodeDChars(name)
	l := fidLen(name)

	if len(buf)/SectorSize != (len(buf)+l-1)/SectorSize {
		pad := (len(buf)/SectorSize+1)*SectorSize - len(buf)
		buf = append(buf, make([]byte, pad)...)
	}

	fid := make([]byte, l)
	le := binary.LittleEndian
	le.PutUint16(fid[16:], 1) // file version number
	var chars uint8
	if name == "" {
		chars |= 0x08 // parent
	}
	if isDir {
		chars |= 0x02 // directory
	}
	fid[18] = chars
	fid[19] = byte(len(dchars))
	copy(fid[20:], longAD(SectorSize, childFE, 0)) // ICB -> child File Entry
	copy(fid[38:], dchars)

	tagLoc := baseLB + uint32(len(buf)/SectorSize)
	putTag(fid, tagFileIdentifier, tagLoc)
	return append(buf, fid...)
}

func encodeDChars(name string) []byte {
	if name == "" {
		return nil
	}
	eightBit := true
	for _, r := range name {
		if r > 0xFF {
			eightBit = false
			break
		}
	}
	if eightBit {
		out := make([]byte, 1+len(name))
		out[0] = 8
		for i := 0; i < len(name); i++ {
			out[1+i] = name[i]
		}
		return out
	}
	u := utf16Encode(name)
	out := make([]byte, 1+2*len(u))
	out[0] = 16
	for i, c := range u {
		binary.BigEndian.PutUint16(out[1+2*i:], c)
	}
	return out
}
