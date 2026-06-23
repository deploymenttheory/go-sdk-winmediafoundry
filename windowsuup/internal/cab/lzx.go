// Package cab implements a pure-Go reader for Microsoft Cabinet (.cab) files,
// including the LZX-compressed variant used by Microsoft's ESD media catalog
// (products.cab) and UUP packages.
//
// The LZX decoder below is adapted from the MIT-licensed WIM LZX decompressor in
// github.com/microsoft/go-winio (wim/lzx/lzx.go, Copyright (c) Microsoft Corp).
// The go-winio implementation targets the WIM variant: a single independent
// 32 KiB chunk, a fixed 32 KiB window, uint16 window indices, the window-15
// position-slot tables, and WIM's fixed E8 (x86) translation convention.
//
// The CAB variant differs and required these changes:
//   - a continuous LZX stream spanning many CFDATA blocks that share window and
//     repeated-offset (R0/R1/R2) state, so indices are uint32 and the window is
//     variable-size (2^windowBits, up to 2 MiB) rather than a fixed 32 KiB;
//   - the full position-slot tables (up to 50 slots for windowBits 21) and
//     support for up to 17 verbatim offset bits;
//   - CAB's E8 translation header (a leading bit, then an optional 32-bit file
//     size) read from the stream and applied per 32 KiB output frame, instead of
//     WIM's hardcoded 12 MB / offset-0 convention.
package cab

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	lzxMainCodeSplit  = 256
	lzxLenCodeCount   = 249
	lzxAlignedSymbols = 8
	lzxPretreeSymbols = 20

	lzxDecodeBits = 16 // flat decode tables are indexed by the next 16 bits
	lzxDecodeSize = 1 << lzxDecodeBits

	lzxFrameSize   = 32768 // E8 translation frame size
	lzxMaxE8Offset = 0x3fffffff
	lzxMaxPosSlots = 50
	lzxMaxMainCode = lzxMainCodeSplit + 8*lzxMaxPosSlots // 656

	lzxVerbatimBlock      = 1
	lzxAlignedOffsetBlock = 2
	lzxUncompressedBlock  = 3
)

// numPositionSlots is indexed by (windowBits - 15); windowBits ranges 15..25.
var numPositionSlots = [...]int{30, 32, 34, 36, 38, 42, 50, 66, 98, 162, 290}

// lzxFooterBits[slot] is the number of "verbatim" offset bits for a position
// slot; lzxBasePosition[slot] is the base offset. Both are generated in init to
// the standard LZX tables (51 entries, extra bits capped at 17).
var (
	lzxFooterBits   [lzxMaxPosSlots + 1]byte
	lzxBasePosition [lzxMaxPosSlots + 1]uint32
)

func init() {
	var pos uint32
	for i := 0; i <= lzxMaxPosSlots; i++ {
		var eb byte
		switch {
		case i < 4:
			eb = 0
		case i < 36:
			eb = byte(i/2 - 1)
		default:
			eb = 17
		}
		lzxFooterBits[i] = eb
		lzxBasePosition[i] = pos
		pos += 1 << eb
	}
}

var (
	errLZXCorrupt        = errors.New("cab: LZX data corrupt")
	errUnsupportedWindow = errors.New("cab: unsupported LZX window size")
)

// lzxDecompressor decodes a continuous CAB LZX stream into a flat output buffer.
type lzxDecompressor struct {
	r   io.Reader
	err error

	// bit reader state (MSB-first over 16-bit little-endian words)
	nbits byte
	c     uint32
	b     []byte
	bv    int
	bo    int

	unaligned bool

	lru          [3]uint32
	mainElements int

	headerRead    bool
	intelFilesize int32

	// Current block state, persisted across CFDATA chunks. A single LZX block
	// can span many 32 KiB chunks; the block header/trees are read once (when
	// blockRemaining reaches 0), while the bit reader is reset per chunk.
	blockType      byte
	blockRemaining uint32

	// Code lengths and flat 16-bit decode tables for the current block's trees.
	mainlens    [lzxMaxMainCode]byte
	lenlens     [lzxLenCodeCount]byte
	alignedlens [lzxAlignedSymbols]byte
	mainTable   []uint16
	lenTable    []uint16
	alnTable    []uint16

	out []byte // flat output buffer; doubles as the sliding window
}

// resetReader points the bit reader at a single CFDATA chunk's compressed bytes
// and clears the bit buffer. CAB LZX restarts the bitstream at each chunk while
// preserving block/window/repeated-offset state.
func (f *lzxDecompressor) resetReader(comp []byte) {
	// Pad so the final getBits near the chunk end never short-reads.
	padded := make([]byte, len(comp)+8)
	copy(padded, comp)
	f.r = bytes.NewReader(padded)
	f.nbits = 0
	f.c = 0
	f.bo = 0
	f.bv = 0
	f.unaligned = false
}

// fail records the first error encountered and empties the read-ahead buffer so
// no further bytes are consumed.
//
//go:noinline
func (f *lzxDecompressor) fail(err error) {
	if f.err == nil {
		f.err = err
	}
	f.bo = 0
	f.bv = 0
}

// ensureAtLeast makes sure the read-ahead buffer holds at least n bytes,
// refilling it from the underlying reader as needed.
func (f *lzxDecompressor) ensureAtLeast(n int) error {
	if f.bv-f.bo >= n {
		return nil
	}
	if f.err != nil {
		return f.err
	}
	if f.bv != f.bo {
		copy(f.b[:f.bv-f.bo], f.b[f.bo:f.bv])
	}
	m, err := io.ReadAtLeast(f.r, f.b[f.bv-f.bo:], n)
	if err != nil {
		if err == io.EOF { //nolint:errorlint
			err = io.ErrUnexpectedEOF
		} else {
			f.fail(err)
		}
		return err
	}
	f.bv = f.bv - f.bo + m
	f.bo = 0
	return nil
}

// feed pulls another 16-bit little-endian word into the bit buffer.
func (f *lzxDecompressor) feed() bool {
	err := f.ensureAtLeast(2)
	if errors.Is(err, io.ErrUnexpectedEOF) { //nolint:errorlint
		return false
	}
	f.c |= (uint32(f.b[f.bo+1])<<8 | uint32(f.b[f.bo])) << (16 - f.nbits)
	f.nbits += 16
	f.bo += 2
	return true
}

// getBits returns the next n bits (n <= 16).
func (f *lzxDecompressor) getBits(n byte) uint16 {
	if f.nbits < n {
		if !f.feed() {
			f.fail(io.ErrUnexpectedEOF)
		}
	}
	c := uint16(f.c >> (32 - n))
	f.c <<= n
	f.nbits -= n
	return c
}

// getBitsLong returns the next n bits for n up to 17, MSB-first.
func (f *lzxDecompressor) getBitsLong(n byte) uint32 {
	if n <= 16 {
		return uint32(f.getBits(n))
	}
	hi := uint32(f.getBits(n - 16))
	lo := uint32(f.getBits(16))
	return hi<<16 | lo
}

// buildLZXTable fills a flat decode table indexed by the next 16 bits: for each
// symbol of code length L, every 16-bit value whose top L bits equal the
// symbol's canonical codeword maps to that symbol. Decoding then reads a symbol
// in O(1) via table[peek16] and consumes lengths[symbol] bits. Returns false if
// the lengths over-subscribe the code space.
func buildLZXTable(lengths []byte, table []uint16) bool {
	var count [17]int
	for _, l := range lengths {
		count[l]++
	}
	var pos [18]int
	for i := 1; i <= 16; i++ {
		pos[i+1] = pos[i] + count[i]<<(16-i)
	}
	for sym, l := range lengths {
		if l == 0 {
			continue
		}
		next := pos[l] + 1<<(16-l)
		if next > lzxDecodeSize {
			return false
		}
		for i := pos[l]; i < next; i++ {
			table[i] = uint16(sym)
		}
		pos[l] = next
	}
	return true
}

// getCode reads one Huffman symbol using a flat decode table and its code
// lengths.
func (f *lzxDecompressor) getCode(table []uint16, lengths []byte) uint16 {
	if f.nbits < 16 {
		f.feed()
	}
	sym := table[f.c>>16]
	n := lengths[sym]
	if n == 0 || f.nbits < n {
		f.fail(errLZXCorrupt)
		return 0
	}
	f.c <<= n
	f.nbits -= n
	return sym
}

// readTree decodes path lengths into lens (prepopulated with the previous
// block's lengths; zero for the first block) using the pretree.
func (f *lzxDecompressor) readTree(lens []byte) error {
	var pretreeLen [lzxPretreeSymbols]byte
	for i := range pretreeLen {
		pretreeLen[i] = byte(f.getBits(4))
	}
	if f.err != nil {
		return f.err
	}
	pretree := make([]uint16, lzxDecodeSize)
	if !buildLZXTable(pretreeLen[:], pretree) {
		return errLZXCorrupt
	}

	for i := 0; i < len(lens); {
		c := byte(f.getCode(pretree, pretreeLen[:]))
		if f.err != nil {
			return f.err
		}
		switch {
		case c <= 16:
			lens[i] = (lens[i] + 17 - c) % 17
			i++
		case c == 17:
			zeroes := int(f.getBits(4)) + 4
			if i+zeroes > len(lens) {
				return errLZXCorrupt
			}
			for j := range zeroes {
				lens[i+j] = 0
			}
			i += zeroes
		case c == 18:
			zeroes := int(f.getBits(5)) + 20
			if i+zeroes > len(lens) {
				return errLZXCorrupt
			}
			for j := range zeroes {
				lens[i+j] = 0
			}
			i += zeroes
		case c == 19:
			same := int(f.getBits(1)) + 4
			if i+same > len(lens) {
				return errLZXCorrupt
			}
			c = byte(f.getCode(pretree, pretreeLen[:]))
			if c > 16 {
				return errLZXCorrupt
			}
			l := (lens[i] + 17 - c) % 17
			for j := range same {
				lens[i+j] = l
			}
			i += same
		default:
			return errLZXCorrupt
		}
	}
	return f.err
}

func (f *lzxDecompressor) readBlockHeader() (blockType byte, blockSize uint32, err error) {
	if f.unaligned {
		if e := f.ensureAtLeast(1); e != nil {
			return 0, 0, e
		}
		f.bo++
		f.unaligned = false
	}

	// CAB LZX block header: 3-bit block type, then a 24-bit uncompressed
	// block length (high 16 bits, then low 8 bits). Unlike the WIM variant,
	// the length is not limited to 32 KiB.
	bt := f.getBits(3)
	hi := uint32(f.getBits(16))
	lo := uint32(f.getBits(8))
	blockSize = hi<<8 | lo
	if f.err != nil {
		return 0, 0, f.err
	}

	switch bt {
	case lzxVerbatimBlock, lzxAlignedOffsetBlock:
		// trees read by caller
	case lzxUncompressedBlock:
		// Align to a 16-bit boundary; if already aligned, a full 16 bits are
		// discarded. The raw data is then read byte-wise, so clear the bit
		// buffer too.
		n := f.nbits
		if n == 0 {
			n = 16
		}
		f.getBits(n)
		f.c = 0
		if e := f.ensureAtLeast(12); e != nil {
			return 0, 0, e
		}
		f.lru[0] = binary.LittleEndian.Uint32(f.b[f.bo : f.bo+4])
		f.lru[1] = binary.LittleEndian.Uint32(f.b[f.bo+4 : f.bo+8])
		f.lru[2] = binary.LittleEndian.Uint32(f.b[f.bo+8 : f.bo+12])
		f.bo += 12
		// An odd-length uncompressed block is followed by one padding byte to
		// restore 16-bit alignment before the next block header.
		f.unaligned = blockSize%2 == 1
	default:
		return 0, 0, errLZXCorrupt
	}
	return byte(bt), blockSize, nil
}

// readTrees reads the Huffman trees for the current block into f.hmain,
// f.hlength and (for aligned blocks) f.haligned.
func (f *lzxDecompressor) readTrees(readAligned bool) error {
	if readAligned {
		for i := range f.alignedlens {
			f.alignedlens[i] = byte(f.getBits(3))
		}
		if !buildLZXTable(f.alignedlens[:], f.alnTable) {
			return errLZXCorrupt
		}
	}

	if err := f.readTree(f.mainlens[:lzxMainCodeSplit]); err != nil {
		return err
	}
	if err := f.readTree(f.mainlens[lzxMainCodeSplit:f.mainElements]); err != nil {
		return err
	}
	if !buildLZXTable(f.mainlens[:f.mainElements], f.mainTable) {
		return errLZXCorrupt
	}

	if err := f.readTree(f.lenlens[:]); err != nil {
		return err
	}
	if !buildLZXTable(f.lenlens[:], f.lenTable) {
		return errLZXCorrupt
	}
	return f.err
}

// decodeMatches decodes literals and matches into f.out[start:end] using the
// current block's trees. end is the lesser of the chunk end and the block end.
func (f *lzxDecompressor) decodeMatches(start, end uint32) (uint32, error) {
	aligned := f.blockType == lzxAlignedOffsetBlock
	i := start
	for i < end {
		main := uint32(f.getCode(f.mainTable, f.mainlens[:f.mainElements]))
		if f.err != nil {
			break
		}
		if main < 256 {
			f.out[i] = byte(main)
			i++
			continue
		}

		matchlen := (main - 256) % 8
		slot := (main - 256) / 8
		if matchlen == 7 {
			matchlen += uint32(f.getCode(f.lenTable, f.lenlens[:]))
		}
		matchlen += 2

		var matchoffset uint32
		if slot < 3 {
			matchoffset = f.lru[slot]
			f.lru[slot] = f.lru[0]
			f.lru[0] = matchoffset
		} else {
			offsetbits := lzxFooterBits[slot]
			var verbatimbits, alignedbits uint32
			if aligned && offsetbits >= 3 {
				verbatimbits = f.getBitsLong(offsetbits-3) * 8
				alignedbits = uint32(f.getCode(f.alnTable, f.alignedlens[:]))
			} else if offsetbits > 0 {
				verbatimbits = f.getBitsLong(offsetbits)
			}
			matchoffset = lzxBasePosition[slot] + verbatimbits + alignedbits - 2
			f.lru[2] = f.lru[1]
			f.lru[1] = f.lru[0]
			f.lru[0] = matchoffset
		}

		if matchoffset > i || matchlen > end-i {
			f.fail(errLZXCorrupt)
			break
		}
		copyend := i + matchlen
		for ; i < copyend; i++ {
			f.out[i] = f.out[i-matchoffset]
		}
	}
	return i - start, f.err
}

// decodeE8 reverses LZX's 0xE8 (x86 CALL) translation over a single output
// frame. off is the frame's absolute output offset; filesize is the translation
// size from the stream header.
func decodeE8(b []byte, off int32, filesize int32) {
	if filesize == 0 || off > lzxMaxE8Offset || len(b) < 10 {
		return
	}
	for i := 0; i < len(b)-10; i++ {
		if b[i] != 0xe8 {
			continue
		}
		currentPtr := off + int32(i)
		abs := int32(binary.LittleEndian.Uint32(b[i+1 : i+5]))
		if abs >= -currentPtr && abs < filesize {
			var rel int32
			if abs >= 0 {
				rel = abs - currentPtr
			} else {
				rel = abs + filesize
			}
			binary.LittleEndian.PutUint32(b[i+1:i+5], uint32(rel))
		}
		i += 4
	}
}

// lzxDecompress decodes the LZX-compressed CFDATA chunks of a CAB folder. Each
// element of chunks is one CFDATA block's compressed bytes; sizes[i] is that
// chunk's uncompressed length (<= 32 KiB). The LZX bitstream restarts at each
// chunk, but block/window/repeated-offset state persists across them.
func lzxDecompress(chunks [][]byte, sizes []int, windowBits int) ([]byte, error) {
	if windowBits < 15 || windowBits-15 >= len(numPositionSlots) {
		return nil, errUnsupportedWindow
	}
	total := 0
	for _, s := range sizes {
		total += s
	}
	f := &lzxDecompressor{
		lru:          [3]uint32{1, 1, 1},
		mainElements: lzxMainCodeSplit + 8*numPositionSlots[windowBits-15],
		b:            make([]byte, 4096),
		out:          make([]byte, total),
		mainTable:    make([]uint16, lzxDecodeSize),
		lenTable:     make([]uint16, lzxDecodeSize),
		alnTable:     make([]uint16, lzxDecodeSize),
	}

	outPos := uint32(0)
	for ci, comp := range chunks {
		f.resetReader(comp)

		// The E8 translation header is read once, at the very start.
		if !f.headerRead {
			if i := f.getBits(1); i != 0 {
				hi := uint32(f.getBits(16))
				lo := uint32(f.getBits(16))
				f.intelFilesize = int32(hi<<16 | lo)
			}
			f.headerRead = true
			if f.err != nil {
				return nil, f.err
			}
		}

		chunkEnd := outPos + uint32(sizes[ci])
		for outPos < chunkEnd {
			if f.blockRemaining == 0 {
				bt, size, err := f.readBlockHeader()
				if err != nil {
					return nil, err
				}
				f.blockType = bt
				f.blockRemaining = size
				if bt != lzxUncompressedBlock {
					if err := f.readTrees(bt == lzxAlignedOffsetBlock); err != nil {
						return nil, err
					}
				}
			}

			limit := min(chunkEnd, outPos+f.blockRemaining)

			var produced uint32
			if f.blockType == lzxUncompressedBlock {
				p, err := f.copyUncompressed(outPos, limit)
				if err != nil {
					return nil, err
				}
				produced = p
			} else {
				p, err := f.decodeMatches(outPos, limit)
				if err != nil {
					return nil, err
				}
				produced = p
			}
			if produced == 0 {
				return nil, errLZXCorrupt
			}
			outPos += produced
			f.blockRemaining -= produced
		}
	}

	// E8 (x86) translation is applied per 32 KiB output frame.
	if f.intelFilesize != 0 {
		for off := 0; off < total; off += lzxFrameSize {
			end := min(off+lzxFrameSize, total)
			decodeE8(f.out[off:end], int32(off), f.intelFilesize)
		}
	}
	return f.out, nil
}

// copyUncompressed handles an uncompressed LZX block, copying raw bytes into
// f.out[start:end]. Bytes already pulled into the read-ahead buffer are consumed
// first, with the remainder read from the underlying reader.
func (f *lzxDecompressor) copyUncompressed(start, end uint32) (uint32, error) {
	pos := start
	if f.bo < f.bv {
		n := min(int(end-pos), f.bv-f.bo)
		copy(f.out[pos:], f.b[f.bo:f.bo+n])
		f.bo += n
		pos += uint32(n)
	}
	if pos < end {
		if _, err := io.ReadFull(f.r, f.out[pos:end]); err != nil {
			return pos - start, fmt.Errorf("cab: uncompressed block: %w", err)
		}
	}
	return end - start, nil
}
