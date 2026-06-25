// Package lzx implements an LZX compressor for WIM resources (encode side).
//
// LZX decode is provided by github.com/Microsoft/go-winio/wim/lzx (already a
// dependency of this module via pkg/wim/decompress.go).  This file adds the
// write side so that the WIM writer can produce LZX-compressed chunks that
// go-winio's lzx.NewReader can decompress, without the XPRESS flag that
// go-winio rejects as unsupported.
//
// Constraints (WIM-specific, matching go-winio's decoder):
//   - Maximum chunk / window size is 32 KiB.
//   - Each chunk is compressed independently (no cross-chunk LZX state).
//   - The decoder always applies E8 (x86 CALL offset) post-processing at
//     offset 0, so the encoder must pre-process accordingly.
//   - Block type: verbatim (1).  Aligned-offset blocks are not used.
package lzx

import (
	"encoding/binary"
	"fmt"
	"sort"
)

// WIM LZX constants.  Values are cross-checked against go-winio's lzx.go
// (maincodecount=496, lencodecount=249, maxBlockSize=32768, e8filesize=12000000).
const (
	windowSize     = 32768
	numMainSyms    = 496 // 256 literals + 30 offset-slots × 8 length-headers
	numLenSyms     = 249 // match lengths 9..257 (encoded as length-code 0..248)
	numPreSyms     = 20  // pre-tree: codes 0-16 = delta, 17-18 = zero-runs, 19 = same-run
	numOffsetSlots = 30  // offset slots for a 32 KiB window
	numLenHdrs     = 8   // 8 length headers per slot (0-6 = primary, 7 = extended)
	minMatch       = 2
	maxMatch       = 257
	maxMainCLen    = 16 // max Huffman code length for main/length trees
	maxPreCLen     = 15 // max Huffman code length for pre-tree (sent as 4-bit raw)
	e8FileSize     = 12_000_000
	hashBits       = 15
	hashSize       = 1 << hashBits
	maxChainLen    = 32 // hash-chain search depth
	niceLen        = 64 // stop looking when we find a match this long
)

// basePos[s] is the base match offset for slot s.
// From go-winio's basePosition table (identical values).
var basePos = [numOffsetSlots + 1]uint32{
	0, 1, 2, 3, 4, 6, 8, 12,
	16, 24, 32, 48, 64, 96, 128, 192,
	256, 384, 512, 768, 1024, 1536, 2048, 3072,
	4096, 6144, 8192, 12288, 16384, 24576, 32768,
}

// extraBits[s] is the number of verbatim extra bits for slot s.
// From go-winio's footerBits table.
var extraBits = [numOffsetSlots]byte{
	0, 0, 0, 0, 1, 1, 2, 2,
	3, 3, 4, 4, 5, 5, 6, 6,
	7, 7, 8, 8, 9, 9, 10, 10,
	11, 11, 12, 12, 13, 13,
}

// Compress compresses in using WIM-compatible LZX and returns the bitstream.
// len(in) must be <= 32768.  The output can be decompressed by
// github.com/Microsoft/go-winio/wim/lzx.NewReader with uncompressedSize=len(in).
func Compress(in []byte) ([]byte, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if len(in) > windowSize {
		return nil, fmt.Errorf("lzx: chunk too large: %d > %d", len(in), windowSize)
	}

	// Work on a copy: E8 preprocessing modifies the data in place.
	buf := make([]byte, len(in))
	copy(buf, in)
	encodeE8(buf)

	toks, mainFreq, lenFreq := lz77Parse(buf)

	mainLens := huffLens(mainFreq[:], numMainSyms, maxMainCLen)
	lenLens := huffLens(lenFreq[:], numLenSyms, maxMainCLen)
	mainCode := canonCodes(mainLens)
	lenCode := canonCodes(lenLens)

	var bw bitWriter
	emitBlock(&bw, len(buf), toks, mainLens, lenLens, mainCode, lenCode)
	return bw.b, nil
}

// ---------------------------------------------------------------------------
// E8 (x86 CALL offset) preprocessing — encoder side.
//
// go-winio's decoder unconditionally applies decodeE8(window, 0) after
// decompressing each chunk.  The encoder must apply the inverse transform so
// that the round-trip is lossless.
//
// Decoder (go-winio decompress.go decodeE8, off=0):
//
//	abs := stored_value
//	if abs >= -currentPtr && abs < e8FileSize {
//	    if abs >= 0 { decoded = abs - currentPtr }
//	    else        { decoded = abs + e8FileSize  }
//	}
//
// Encoder inverse (produces the stored_value that the decoder turns back into
// the original):
//
//	For original >= 0:  store abs = original + currentPtr  (if in [0, e8FileSize))
//	For original < 0:   store abs = original - e8FileSize  (if in [-currentPtr, 0))
// ---------------------------------------------------------------------------
func encodeE8(b []byte) {
	if len(b) < 10 {
		return
	}
	limit := len(b) - 10
	for i := 0; i <= limit; i++ {
		if b[i] != 0xe8 {
			continue
		}
		cur := int32(i)
		orig := int32(binary.LittleEndian.Uint32(b[i+1:]))
		var abs int32
		if orig >= 0 {
			abs = orig + cur
			if abs >= 0 && abs < int32(e8FileSize) {
				binary.LittleEndian.PutUint32(b[i+1:], uint32(abs))
			}
		} else {
			abs = orig - int32(e8FileSize)
			if abs >= -cur && abs < 0 {
				binary.LittleEndian.PutUint32(b[i+1:], uint32(abs))
			}
		}
		i += 4 // skip the 4-byte operand (matches decoder's skip)
	}
}

// ---------------------------------------------------------------------------
// LZ77 greedy parser.
// ---------------------------------------------------------------------------

type token struct {
	isMatch bool
	lit     byte
	offset  int // match offset (matchoffset in decoder terms: 1 = prev byte)
	length  int // match length
}

func lz77Parse(data []byte) ([]token, [numMainSyms]uint32, [numLenSyms]uint32) {
	n := len(data)
	toks := make([]token, 0, n/4+16)
	var mainFreq [numMainSyms]uint32
	var lenFreq [numLenSyms]uint32

	head := make([]int32, hashSize)
	for i := range head {
		head[i] = -1
	}
	prev := make([]int32, n)

	hash3 := func(pos int) uint32 {
		v := uint32(data[pos]) | uint32(data[pos+1])<<8 | uint32(data[pos+2])<<16
		return (v * 0x9E3779B1) >> (32 - hashBits)
	}

	insert := func(pos int) {
		if pos+2 >= n {
			return
		}
		h := hash3(pos)
		prev[pos] = head[h]
		head[h] = int32(pos)
	}

	findMatch := func(pos int) (bestLen, bestOff int) {
		if pos+minMatch > n {
			return 0, 0
		}
		avail := n - pos
		if avail > maxMatch {
			avail = maxMatch
		}
		if pos+2 >= n {
			return 0, 0
		}
		h := hash3(pos)
		cur := head[h]
		chain := maxChainLen
		bestLen = minMatch - 1
		for cur >= 0 && chain > 0 {
			chain--
			off := pos - int(cur)
			if off > windowSize {
				break
			}
			// Quick reject: byte just past current best.
			if bestLen < avail && data[int(cur)+bestLen] == data[pos+bestLen] {
				l := 0
				for l < avail && data[int(cur)+l] == data[pos+l] {
					l++
				}
				if l > bestLen {
					bestLen = l
					bestOff = off
					if l >= niceLen || l == avail {
						break
					}
				}
			}
			cur = prev[cur]
		}
		if bestLen < minMatch {
			return 0, 0
		}
		return bestLen, bestOff
	}

	for i := 0; i < n; {
		l, off := findMatch(i)
		if l >= minMatch {
			slot := offsetSlot(off)
			lh := l - minMatch
			if lh >= numLenHdrs-1 {
				lh = numLenHdrs - 1
			}
			mainFreq[256+slot*numLenHdrs+lh]++
			if l-minMatch >= numLenHdrs-1 {
				lc := l - minMatch - (numLenHdrs - 1)
				lenFreq[lc]++
			}
			toks = append(toks, token{isMatch: true, offset: off, length: l})
			for j := 0; j < l; j++ {
				insert(i + j)
			}
			i += l
		} else {
			insert(i)
			mainFreq[data[i]]++
			toks = append(toks, token{lit: data[i]})
			i++
		}
	}
	return toks, mainFreq, lenFreq
}

// offsetSlot returns the LZX offset slot for a match offset.
// Relationship: matchoffset = basePos[slot] + extraBits_value - 2
// So: slot is the largest s where basePos[s]-2 <= offset < basePos[s+1]-2,
// i.e. basePos[s] <= offset+2 < basePos[s+1].
func offsetSlot(offset int) int {
	adj := uint32(offset + 2)
	for s := 3; s < numOffsetSlots-1; s++ {
		if basePos[s+1] > adj {
			return s
		}
	}
	return numOffsetSlots - 1
}

// extraBitsVal returns the extra-bits value for a given offset and its slot.
func extraBitsVal(offset, slot int) int {
	return int(uint32(offset+2) - basePos[slot])
}

// ---------------------------------------------------------------------------
// Huffman code builder.
// ---------------------------------------------------------------------------

type symFreqEntry struct {
	sym  int
	freq uint32
}

// huffLens computes length-limited Huffman code lengths for the first count
// symbols of freqs.  Symbols with zero frequency get length 0.
func huffLens(freqs []uint32, count, maxLen int) []byte {
	lens := make([]byte, count)
	var used []symFreqEntry
	for i := 0; i < count; i++ {
		if freqs[i] > 0 {
			used = append(used, symFreqEntry{i, freqs[i]})
		}
	}
	switch len(used) {
	case 0:
		return lens
	case 1:
		lens[used[0].sym] = 1
		return lens
	}

	raw := huffCodeLengths(used, count)
	limitLens(raw, used, maxLen)
	for s, l := range raw {
		if s < count {
			lens[s] = byte(l)
		}
	}
	return lens
}

// huffCodeLengths returns optimal (unbounded) code lengths for used symbols.
// count is the alphabet size; the returned slice has length count.
func huffCodeLengths(used []symFreqEntry, count int) []int {
	lens := make([]int, count)

	type node struct {
		freq        uint32
		left, right int
		sym         int
	}
	nodes := make([]node, 0, 2*len(used))
	for _, u := range used {
		nodes = append(nodes, node{freq: u.freq, left: -1, right: -1, sym: u.sym})
	}

	heap := make([]int, len(nodes))
	for i := range heap {
		heap[i] = i
	}
	less := func(a, b int) bool {
		if nodes[a].freq != nodes[b].freq {
			return nodes[a].freq < nodes[b].freq
		}
		return a < b
	}
	sort.Slice(heap, func(i, j int) bool { return less(heap[i], heap[j]) })

	pop := func() int {
		v := heap[0]
		heap = heap[1:]
		return v
	}
	push := func(idx int) {
		pos := sort.Search(len(heap), func(i int) bool { return !less(heap[i], idx) })
		heap = append(heap, 0)
		copy(heap[pos+1:], heap[pos:])
		heap[pos] = idx
	}

	for len(heap) > 1 {
		a, b := pop(), pop()
		nodes = append(nodes, node{freq: nodes[a].freq + nodes[b].freq, left: a, right: b, sym: -1})
		push(len(nodes) - 1)
	}

	var walk func(idx, depth int)
	walk = func(idx, depth int) {
		nd := nodes[idx]
		if nd.sym >= 0 {
			if depth == 0 {
				depth = 1
			}
			lens[nd.sym] = depth
			return
		}
		walk(nd.left, depth+1)
		walk(nd.right, depth+1)
	}
	walk(heap[0], 0)
	return lens
}

// limitLens enforces maxLen using the classic Kraft-repair approach.
func limitLens(lens []int, used []symFreqEntry, maxLen int) {
	for _, u := range used {
		if lens[u.sym] > maxLen {
			lens[u.sym] = maxLen
		}
	}
	one := 1 << maxLen
	kraft := 0
	for _, u := range used {
		kraft += one >> lens[u.sym]
	}
	if kraft <= one {
		return
	}
	order := make([]int, len(used))
	for i, u := range used {
		order[i] = u.sym
	}
	sort.Slice(order, func(i, j int) bool {
		if lens[order[i]] != lens[order[j]] {
			return lens[order[i]] < lens[order[j]]
		}
		return order[i] < order[j]
	})
	for kraft > one {
		progressed := false
		for _, s := range order {
			if kraft <= one {
				break
			}
			if lens[s] < maxLen {
				kraft -= one >> lens[s]
				lens[s]++
				kraft += one >> lens[s]
				progressed = true
			}
		}
		if !progressed {
			break
		}
		sort.Slice(order, func(i, j int) bool {
			if lens[order[i]] != lens[order[j]] {
				return lens[order[i]] < lens[order[j]]
			}
			return order[i] < order[j]
		})
	}
}

// canonCodes assigns canonical codewords from code lengths.
// The assignment order matches go-winio's buildTable: (length asc, symbol asc).
func canonCodes(lens []byte) []uint32 {
	codes := make([]uint32, len(lens))
	code := 0
	for length := 1; length <= maxMainCLen; length++ {
		for sym := 0; sym < len(lens); sym++ {
			if int(lens[sym]) != length {
				continue
			}
			codes[sym] = uint32(code)
			code++
		}
		code <<= 1
	}
	return codes
}

// ---------------------------------------------------------------------------
// LZX bit-packer.
//
// LZX stores bits MSB-first in 16-bit little-endian words.  The decoder reads
// them as:
//
//	f.c |= (uint32(b[1])<<8 | uint32(b[0])) << (16 - f.nbits)
//
// So the encoder packs: MSB of the logical code → bit 31 of the accumulator →
// first bit of the first LE word emitted.
// ---------------------------------------------------------------------------

type bitWriter struct {
	b   []byte
	acc uint32
	cnt uint
}

func (w *bitWriter) putBits(val uint32, n uint) {
	w.acc |= val << (32 - w.cnt - n)
	w.cnt += n
	for w.cnt >= 16 {
		u := uint16(w.acc >> 16)
		w.b = append(w.b, byte(u), byte(u>>8))
		w.acc <<= 16
		w.cnt -= 16
	}
}

// flush pads the remaining partial word with zero bits, writes it, then
// appends two extra zero words.  go-winio's LZX decoder pre-fetches one
// 16-bit word whenever f.nbits < maxTreePathLen (16), even when it already
// has enough bits for the symbol it is about to decode.  Without the extra
// words that pre-fetch hits real EOF while valid bits are still in the
// accumulator, poisoning f.err and causing "unexpected EOF" on the very next
// error check.  Two extra zero words (4 bytes) give the decoder headroom for
// up to two consecutive pre-fetches after the last real data bit.
func (w *bitWriter) flush() {
	if w.cnt > 0 {
		u := uint16(w.acc >> 16)
		w.b = append(w.b, byte(u), byte(u>>8))
		w.acc = 0
		w.cnt = 0
	}
	w.b = append(w.b, 0, 0, 0, 0)
}

// ---------------------------------------------------------------------------
// Block emission.
// ---------------------------------------------------------------------------

// emitBlock writes one LZX verbatim block covering the entire chunk.
func emitBlock(bw *bitWriter, dataLen int, toks []token, mainLens, lenLens []byte, mainCode, lenCode []uint32) {
	// Block type 1 = verbatim.
	bw.putBits(1, 3)

	// Full-block flag + optional size.
	if dataLen == windowSize {
		bw.putBits(1, 1)
	} else {
		bw.putBits(0, 1)
		bw.putBits(uint32(dataLen), 16)
	}

	// Main tree: go-winio reads it in two halves.
	var prevMain [numMainSyms]byte // all zeros: first block in chunk
	emitTree(bw, mainLens[:256], prevMain[:256])
	emitTree(bw, mainLens[256:], prevMain[256:])

	// Length tree.
	var prevLen [numLenSyms]byte
	emitTree(bw, lenLens, prevLen[:])

	// Token stream.
	for _, t := range toks {
		if !t.isMatch {
			bw.putBits(mainCode[t.lit], uint(mainLens[t.lit]))
			continue
		}
		slot := offsetSlot(t.offset)
		eb := extraBitsVal(t.offset, slot)
		lh := t.length - minMatch
		if lh >= numLenHdrs-1 {
			lh = numLenHdrs - 1
		}
		ms := 256 + slot*numLenHdrs + lh
		bw.putBits(mainCode[ms], uint(mainLens[ms]))
		// go-winio (and the LZX spec) reads: main → length code → extra offset bits.
		// The length code must be written before the verbatim offset bits.
		if t.length-minMatch >= numLenHdrs-1 {
			lc := t.length - minMatch - (numLenHdrs - 1)
			bw.putBits(lenCode[lc], uint(lenLens[lc]))
		}
		if eb_bits := uint(extraBits[slot]); eb_bits > 0 {
			bw.putBits(uint32(eb), eb_bits)
		}
	}
	bw.flush()
}

// emitTree transmits one tree half using a pre-tree, as expected by
// go-winio's readTree.  newLens and prevLens must have the same length.
func emitTree(bw *bitWriter, newLens, prevLens []byte) {
	count := len(newLens)

	// Build delta codes.
	delta := make([]int, count)
	for i := 0; i < count; i++ {
		delta[i] = int((int(prevLens[i])+17-int(newLens[i]))%17)
	}

	// Count pre-tree symbol frequencies (codes 0-18; code 19 not used).
	var preFreq [numPreSyms]uint32
	for i := 0; i < count; {
		d := delta[i]
		if d == 0 {
			// Count the zero run.
			j := i
			for j < count && delta[j] == 0 {
				j++
			}
			run := j - i
			for run >= 20 {
				n := run
				if n > 51 {
					n = 51 // code 18 param: 0..31 → run 20..51
				}
				preFreq[18]++
				run -= n
				i += n
			}
			for run >= 4 {
				n := run
				if n > 19 {
					n = 19 // code 17 param: 0..15 → run 4..19
				}
				preFreq[17]++
				run -= n
				i += n
			}
			for ; run > 0; run-- {
				preFreq[0]++ // delta 0 encoded as code 0
				i++
			}
		} else {
			preFreq[d]++
			i++
		}
	}

	// Ensure the pre-tree always has at least two symbols so go-winio's
	// buildTable (which requires Kraft sum == 1) can construct a valid table.
	// If only one symbol is used, introduce a neighbour with frequency 1.  The
	// dummy symbol is included in the transmitted pre-tree lengths but is never
	// emitted in the delta stream, so the decoder reconstructs the correct tree.
	{
		nz, lastNZ := 0, -1
		for i, f := range preFreq {
			if f > 0 {
				nz++
				lastNZ = i
			}
		}
		if nz <= 1 {
			dummy := 0
			if lastNZ == 0 {
				dummy = 1
			}
			preFreq[dummy]++
		}
	}

	// Build pre-tree.
	preLens := huffLens(preFreq[:], numPreSyms, maxPreCLen)
	preCodes := canonCodes(preLens)

	// Emit pre-tree lengths as raw 4-bit values (go-winio reads getBits(4) × 20).
	for _, l := range preLens {
		bw.putBits(uint32(l), 4)
	}

	// Emit delta codes using the pre-tree.
	for i := 0; i < count; {
		d := delta[i]
		if d == 0 {
			j := i
			for j < count && delta[j] == 0 {
				j++
			}
			run := j - i
			for run >= 20 {
				n := run
				if n > 51 {
					n = 51
				}
				bw.putBits(preCodes[18], uint(preLens[18]))
				bw.putBits(uint32(n-20), 5)
				run -= n
				i += n
			}
			for run >= 4 {
				n := run
				if n > 19 {
					n = 19
				}
				bw.putBits(preCodes[17], uint(preLens[17]))
				bw.putBits(uint32(n-4), 4)
				run -= n
				i += n
			}
			for ; run > 0; run-- {
				bw.putBits(preCodes[0], uint(preLens[0]))
				i++
			}
		} else {
			bw.putBits(preCodes[d], uint(preLens[d]))
			i++
		}
	}
}
