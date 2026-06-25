package lzx_test

// debugDecompress is a self-contained copy of go-winio's WIM LZX decoder
// (github.com/Microsoft/go-winio/wim/lzx) instrumented with t.Logf calls so
// that test failures can be diagnosed without a debugger.  The production
// round-trip tests in compress_test.go use the real go-winio decoder; this
// file adds a parallel debugRoundTrip path that logs every tree read, every
// token decoded, and the exact point of stream corruption.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"testing"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/pkg/wim/lzx"
)

// ── constants mirrored from go-winio lzx.go ─────────────────────────────────

const (
	dbgMainCodeCount = 496
	dbgMainCodeSplit = 256
	dbgLenCodeCount  = 249
	dbgLenShift      = 9
	dbgCodeMask      = 0x1ff
	dbgTableBits     = 9
	dbgTableSize     = 1 << dbgTableBits
	dbgMaxTreePath   = 16
	dbgMaxBlockSize  = 32768
	dbgE8FileSize    = 12_000_000
	dbgVerbatimBlock = 1
	dbgAlignedBlock  = 2
	dbgUncompressed  = 3
)

var dbgFooterBits = [...]byte{
	0, 0, 0, 0, 1, 1, 2, 2,
	3, 3, 4, 4, 5, 5, 6, 6,
	7, 7, 8, 8, 9, 9, 10, 10,
	11, 11, 12, 12, 13, 13, 14,
}

var dbgBasePosition = [...]uint16{
	0, 1, 2, 3, 4, 6, 8, 12,
	16, 24, 32, 48, 64, 96, 128, 192,
	256, 384, 512, 768, 1024, 1536, 2048, 3072,
	4096, 6144, 8192, 12288, 16384, 24576, 32768,
}

// ── Huffman table ────────────────────────────────────────────────────────────

type dbgHuffman struct {
	extra   [][]uint16
	maxbits byte
	table   [dbgTableSize]uint16
}

func dbgBuildTable(codelens []byte) *dbgHuffman {
	var count [dbgMaxTreePath + 1]uint
	var maxValue byte
	for _, cl := range codelens {
		count[cl]++
		if maxValue < cl {
			maxValue = cl
		}
	}
	if maxValue == 0 {
		return &dbgHuffman{}
	}
	var first [dbgMaxTreePath + 1]uint
	code := uint(0)
	for i := byte(1); i <= maxValue; i++ {
		code <<= 1
		first[i] = code
		code += count[i]
	}
	if code != 1<<maxValue {
		return nil // incomplete or over-complete code
	}
	h := &dbgHuffman{maxbits: maxValue}
	if maxValue > dbgTableBits {
		core := first[dbgTableBits+1] / 2
		nextra := 1<<dbgTableBits - core
		h.extra = make([][]uint16, nextra)
		for c := core; c < 1<<dbgTableBits; c++ {
			h.table[c] = uint16(c - core)
			h.extra[c-core] = make([]uint16, 1<<(maxValue-dbgTableBits))
		}
	}
	for i, cl := range codelens {
		if cl != 0 {
			c := first[cl]
			first[cl]++
			v := uint16(cl)<<dbgLenShift | uint16(i)
			if cl <= dbgTableBits {
				ext := c << (dbgTableBits - cl)
				for j := uint(0); j < 1<<(dbgTableBits-cl); j++ {
					h.table[ext+j] = v
				}
			} else {
				prefix := c >> (cl - dbgTableBits)
				suffix := c & (1<<(cl-dbgTableBits) - 1)
				ext := suffix << (maxValue - cl)
				for j := uint(0); j < 1<<(maxValue-cl); j++ {
					h.extra[h.table[prefix]][ext+j] = v
				}
			}
		}
	}
	return h
}

// ── Instrumented decompressor ────────────────────────────────────────────────

type dbgDecompressor struct {
	t            *testing.T
	r            io.Reader
	err          error
	nbits        byte
	c            uint32
	lru          [3]uint16
	uncompressed int
	mainlens     [dbgMainCodeCount]byte
	lenlens      [dbgLenCodeCount]byte
	window       [dbgMaxBlockSize]byte
	b            []byte
	bv           int
	bo           int
	bitsConsumed int // total bits consumed (for stream position reporting)
}

func (f *dbgDecompressor) fail(err error) {
	if f.err == nil {
		f.err = err
	}
}

func (f *dbgDecompressor) ensureAtLeast(n int) error {
	if f.bv-f.bo >= n {
		return nil
	}
	if f.err != nil {
		return f.err
	}
	if f.bv != f.bo {
		copy(f.b[:f.bv-f.bo], f.b[f.bo:f.bv])
	}
	nr, err := io.ReadAtLeast(f.r, f.b[f.bv-f.bo:], n-(f.bv-f.bo))
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		f.fail(err)
		return err
	}
	f.bv = f.bv - f.bo + nr
	f.bo = 0
	return nil
}

func (f *dbgDecompressor) feed() bool {
	if err := f.ensureAtLeast(2); err == io.ErrUnexpectedEOF { //nolint:errorlint
		return false
	}
	f.c |= (uint32(f.b[f.bo+1])<<8 | uint32(f.b[f.bo])) << (16 - f.nbits)
	f.nbits += 16
	f.bo += 2
	return true
}

func (f *dbgDecompressor) getBits(n byte) uint16 {
	if f.nbits < n {
		if !f.feed() {
			f.fail(io.ErrUnexpectedEOF)
		}
	}
	c := uint16(f.c >> (32 - n))
	f.c <<= n
	f.nbits -= n
	f.bitsConsumed += int(n)
	return c
}

func (f *dbgDecompressor) getCode(h *dbgHuffman) uint16 {
	if h.maxbits > 0 {
		if f.nbits < dbgMaxTreePath {
			f.feed()
		}
		c := h.table[f.c>>(32-dbgTableBits)]
		if !(c >= 1<<dbgLenShift) {
			c = h.extra[c][f.c<<dbgTableBits>>(32-(h.maxbits-dbgTableBits))]
		}
		n := byte(c >> dbgLenShift)
		if f.nbits >= n {
			f.c <<= n
			f.nbits -= n
			f.bitsConsumed += int(n)
			return c & dbgCodeMask
		}
		f.fail(io.ErrUnexpectedEOF)
		return 0
	}
	f.t.Logf("  getCode: empty huffman tree (maxbits==0) at bit %d", f.bitsConsumed)
	f.fail(fmt.Errorf("LZX data corrupt: empty huffman table"))
	return 0
}

func (f *dbgDecompressor) readTree(lens []byte, name string) error {
	var pretreeLen [20]byte
	for i := range pretreeLen {
		pretreeLen[i] = byte(f.getBits(4))
	}
	if f.err != nil {
		return f.err
	}

	f.t.Logf("  readTree(%s): pre-tree lengths = %v", name, pretreeLen)

	h := dbgBuildTable(pretreeLen[:])
	if h == nil {
		return fmt.Errorf("readTree(%s): buildTable returned nil (invalid pre-tree at bit %d)", name, f.bitsConsumed)
	}
	if h.maxbits == 0 {
		return fmt.Errorf("readTree(%s): pre-tree is empty (all zero lengths)", name)
	}

	var nonZero int
	for _, l := range pretreeLen {
		if l != 0 {
			nonZero++
		}
	}
	f.t.Logf("  readTree(%s): pre-tree has %d non-zero symbols, maxbits=%d", name, nonZero, h.maxbits)

	for i := 0; i < len(lens); {
		c := byte(f.getCode(h))
		if f.err != nil {
			return f.err
		}
		switch {
		case c <= 16:
			prev := lens[i]
			lens[i] = (lens[i] + 17 - c) % 17
			if lens[i] != 0 {
				f.t.Logf("    [%s] sym[%3d]: delta=%d  %d->%d  (bit %d)", name, i, c, prev, lens[i], f.bitsConsumed)
			}
			i++
		case c == 17:
			zeroes := int(f.getBits(4)) + 4
			if i+zeroes > len(lens) {
				return fmt.Errorf("readTree(%s): code 17 run overflows at i=%d zeroes=%d len=%d", name, i, zeroes, len(lens))
			}
			for j := 0; j < zeroes; j++ {
				lens[i+j] = 0
			}
			i += zeroes
		case c == 18:
			zeroes := int(f.getBits(5)) + 20
			if i+zeroes > len(lens) {
				return fmt.Errorf("readTree(%s): code 18 run overflows at i=%d zeroes=%d len=%d", name, i, zeroes, len(lens))
			}
			for j := 0; j < zeroes; j++ {
				lens[i+j] = 0
			}
			i += zeroes
		case c == 19:
			same := int(f.getBits(1)) + 4
			if i+same > len(lens) {
				return fmt.Errorf("readTree(%s): code 19 run overflows at i=%d same=%d len=%d", name, i, same, len(lens))
			}
			c = byte(f.getCode(h))
			if c > 16 {
				return fmt.Errorf("readTree(%s): code 19 inner code=%d > 16", name, c)
			}
			l := (lens[i] + 17 - c) % 17
			f.t.Logf("    [%s] code19 run=%d l=%d (bit %d)", name, same, l, f.bitsConsumed)
			for j := 0; j < same; j++ {
				lens[i+j] = l
			}
			i += same
		default:
			return fmt.Errorf("readTree(%s): invalid pre-tree code %d at i=%d bit %d", name, c, i, f.bitsConsumed)
		}
	}
	return f.err
}

func (f *dbgDecompressor) decompress() error {
	f.t.Logf("decompress: uncompressedSize=%d", f.uncompressed)
	n := 0
	for n < f.uncompressed {
		k, err := f.readBlock(uint16(n))
		if err != nil {
			return fmt.Errorf("readBlock(start=%d): %w", n, err)
		}
		n += k
	}
	f.t.Logf("decompress: decoded %d bytes, applying E8", n)
	dbgDecodeE8(f.window[:f.uncompressed], 0)
	return nil
}

func (f *dbgDecompressor) readBlock(start uint16) (int, error) {
	blockType := f.getBits(3)
	full := f.getBits(1)
	var blockSize uint16
	if full != 0 {
		blockSize = dbgMaxBlockSize
	} else {
		blockSize = f.getBits(16)
		if blockSize > dbgMaxBlockSize {
			return 0, fmt.Errorf("blockSize %d > maxBlockSize at bit %d", blockSize, f.bitsConsumed)
		}
	}
	if f.err != nil {
		return 0, f.err
	}
	f.t.Logf("readBlock: start=%d blockType=%d full=%d blockSize=%d (header end bit=%d)", start, blockType, full, blockSize, f.bitsConsumed)

	if blockType != dbgVerbatimBlock && blockType != dbgAlignedBlock && blockType != dbgUncompressed {
		return 0, fmt.Errorf("unknown blockType=%d at bit %d", blockType, f.bitsConsumed)
	}
	if blockType == dbgUncompressed {
		return 0, fmt.Errorf("uncompressed blocks not expected in this test path")
	}

	// Main tree (two halves).
	if err := f.readTree(f.mainlens[:dbgMainCodeSplit], "main[0:256]"); err != nil {
		return 0, err
	}
	if err := f.readTree(f.mainlens[dbgMainCodeSplit:], "main[256:496]"); err != nil {
		return 0, err
	}

	// Summarise non-zero main tree lengths.
	var mainNZ []string
	for i, l := range f.mainlens {
		if l != 0 {
			mainNZ = append(mainNZ, fmt.Sprintf("%d:%d", i, l))
		}
	}
	f.t.Logf("  main tree: %d non-zero symbols: %v", len(mainNZ), mainNZ)

	main := dbgBuildTable(f.mainlens[:])
	if main == nil {
		return 0, fmt.Errorf("buildTable(main) returned nil at bit %d", f.bitsConsumed)
	}

	// Length tree.
	if err := f.readTree(f.lenlens[:], "len"); err != nil {
		return 0, err
	}
	var lenNZ []string
	for i, l := range f.lenlens {
		if l != 0 {
			lenNZ = append(lenNZ, fmt.Sprintf("%d:%d", i, l))
		}
	}
	f.t.Logf("  len tree: %d non-zero symbols: %v", len(lenNZ), lenNZ)

	length := dbgBuildTable(f.lenlens[:])
	if length == nil {
		return 0, fmt.Errorf("buildTable(len) returned nil at bit %d", f.bitsConsumed)
	}

	return f.readCompressedBlock(start, start+blockSize, main, length, f.bitsConsumed)
}

func (f *dbgDecompressor) readCompressedBlock(start, end uint16, hmain, hlength *dbgHuffman, treeBit int) (int, error) {
	f.t.Logf("  readCompressedBlock: start=%d end=%d (token stream starts at bit %d)", start, end, treeBit)
	f.t.Logf("  initial state: f.c=0x%08x f.nbits=%d", f.c, f.nbits)
	i := start
	tokenCount := 0
	for i < end {
		// Log state at entry of each token so we can spot the misalignment.
		startBit := f.bitsConsumed
		startNbits := f.nbits
		startC := f.c

		mainSym := f.getCode(hmain)
		if f.err != nil {
			return int(i - start), fmt.Errorf("getCode(main) failed at i=%d token=%d: %w", i, tokenCount, f.err)
		}
		tokenCount++
		if mainSym < 256 {
			f.window[i] = byte(mainSym)
			i++
			continue
		}

		matchlenHdr := (mainSym - 256) % 8
		slot := (mainSym - 256) / 8

		// Log each match token so we can compare against the encoder.
		f.t.Logf("  tok=%d i=%d bit_entry=%d(nbits=%d c=0x%08x) mainSym=%d slot=%d lhdr=%d  →after_main: bit=%d nbits=%d c=0x%08x",
			tokenCount, i, startBit, startNbits, startC, mainSym, slot, matchlenHdr, f.bitsConsumed, f.nbits, f.c)

		matchlen := matchlenHdr
		if matchlenHdr == 7 {
			lenSym := f.getCode(hlength)
			if f.err != nil {
				return int(i - start), fmt.Errorf("getCode(len) failed at i=%d token=%d slot=%d: %w", i, tokenCount, slot, f.err)
			}
			matchlen += lenSym
		}
		matchlen += 2

		var matchoffset uint16
		if slot < 3 {
			matchoffset = f.lru[slot]
			f.lru[slot] = f.lru[0]
			f.lru[0] = matchoffset
		} else {
			offsetbits := dbgFooterBits[slot]
			var verbatimbits uint16
			beforeExtra := f.bitsConsumed
			if offsetbits > 0 {
				verbatimbits = f.getBits(offsetbits)
				if f.err != nil {
					return int(i - start), fmt.Errorf("getBits(offsetbits=%d) failed at i=%d token=%d: %w", offsetbits, i, tokenCount, f.err)
				}
			}
			matchoffset = dbgBasePosition[slot] + verbatimbits - 2
			f.t.Logf("    extra: slot=%d offsetbits=%d verbatimbits=%d matchoffset=%d (bits %d..%d) nbits=%d c=0x%08x",
				slot, offsetbits, verbatimbits, matchoffset, beforeExtra, f.bitsConsumed-1, f.nbits, f.c)
			f.lru[2] = f.lru[1]
			f.lru[1] = f.lru[0]
			f.lru[0] = matchoffset
		}

		if !(matchoffset <= i && matchlen <= end-i) {
			f.t.Logf("  CORRUPT at token=%d i=%d: matchoffset=%d i=%d matchlen=%d end-i=%d slot=%d mainSym=%d",
				tokenCount, i, matchoffset, i, matchlen, end-i, slot, mainSym)
			return int(i - start), fmt.Errorf("LZX data corrupt: matchoffset=%d > i=%d OR matchlen=%d > end-i=%d (token=%d slot=%d)",
				matchoffset, i, matchlen, end-i, tokenCount, slot)
		}

		copyend := i + matchlen
		for ; i < copyend; i++ {
			f.window[i] = f.window[i-matchoffset]
		}
	}
	f.t.Logf("  readCompressedBlock: decoded %d bytes (%d tokens)", int(i-start), tokenCount)
	return int(i - start), f.err
}

func dbgDecodeE8(b []byte, off int64) {
	if len(b) < 10 {
		return
	}
	for i := 0; i < len(b)-10; i++ {
		if b[i] == 0xe8 {
			currentPtr := int32(off) + int32(i)
			abs := int32(binary.LittleEndian.Uint32(b[i+1 : i+5]))
			if abs >= -currentPtr && abs < dbgE8FileSize {
				var rel int32
				if abs >= 0 {
					rel = abs - currentPtr
				} else {
					rel = abs + dbgE8FileSize
				}
				binary.LittleEndian.PutUint32(b[i+1:i+5], uint32(rel))
			}
			i += 4
		}
	}
}

// debugRoundTrip runs lzx.Compress then the instrumented decoder, logging
// the full decode trace on any failure.
func debugRoundTrip(t *testing.T, in []byte) {
	t.Helper()
	compressed, err := lzx.Compress(in)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	t.Logf("debugRoundTrip: in=%d bytes  compressed=%d bytes", len(in), len(compressed))
	if len(compressed) >= len(in) {
		t.Logf("  (no compression gain, skipping decode)")
		return
	}

	// Hex dump the compressed bytes so we can hand-trace the bitstream.
	{
		var sb bytes.Buffer
		for idx, byt := range compressed {
			if idx%16 == 0 {
				fmt.Fprintf(&sb, "\n  %04x: ", idx)
			}
			fmt.Fprintf(&sb, "%02x ", byt)
		}
		t.Logf("compressed hex (%d bytes):%s", len(compressed), sb.String())
	}

	d := &dbgDecompressor{
		t:            t,
		lru:          [3]uint16{1, 1, 1},
		uncompressed: len(in),
		b:            make([]byte, 4096),
		r:            bytes.NewReader(compressed),
	}
	if err := d.decompress(); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := d.window[:len(in)]
	if !bytes.Equal(got, in) {
		// Find first mismatch.
		for j := range in {
			if j >= len(got) || got[j] != in[j] {
				t.Fatalf("round-trip mismatch at byte %d: got 0x%02x want 0x%02x", j, got[j], in[j])
			}
		}
		t.Fatalf("round-trip mismatch: len(in)=%d len(got)=%d", len(in), len(got))
	}
	t.Logf("  OK: round-trip verified")
}

// ── Debug test entry-points ──────────────────────────────────────────────────

func TestDebugPartialChunk(t *testing.T) {
	debugRoundTrip(t, bytes.Repeat([]byte("abcdefgh"), 1000))
}

func TestDebugFull32K(t *testing.T) {
	buf := bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog.\n"), 750)
	debugRoundTrip(t, buf[:32768])
}

func TestDebugE8Bytes(t *testing.T) {
	buf := make([]byte, 16384)
	for i := 0; i < len(buf); i += 8 {
		buf[i] = 0xe8
		buf[i+1] = byte(i)
		buf[i+2] = byte(i >> 8)
		buf[i+3] = 0
		buf[i+4] = 0
	}
	debugRoundTrip(t, buf)
}

func TestDebugAllZeros(t *testing.T) {
	debugRoundTrip(t, make([]byte, 32768))
}
