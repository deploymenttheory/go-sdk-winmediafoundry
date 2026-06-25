// Package xpress also implements the XPRESS (LZ77 + Huffman) compressor, the
// counterpart to Decompress. Compress emits chunks that Decompress reads back
// byte-for-byte: a 256-byte codeword-length header followed by the interleaved
// 16-bit-little-endian bitstream and raw length-overflow bytes, mirroring
// wimlib's xpress_compress.c output_bitstream exactly.
package xpress

import (
	"math/bits"
	"sort"
)

const (
	// Match finder tuning.
	hashBits    = 15
	hashSize    = 1 << hashBits
	maxMatchLen = 65536 // a chunk is at most 64 KiB
	maxChainLen = 32    // hash-chain search depth (speed/ratio trade-off)
	niceMatch   = 64    // stop searching once a match this long is found
)

// Compress compresses a single XPRESS chunk (len(in) up to 65536; WIM uses
// 32768). The result, passed to Decompress with len(in) as outSize, reproduces
// in exactly.
func Compress(in []byte) ([]byte, error) {
	// Pass 1: LZ77-parse to gather the symbol frequencies.
	tokens, freqs := lz77Parse(in)

	// Build a length-limited canonical Huffman code (max length 15) matching the
	// decoder's canonical assignment. Every symbol that appears must get a
	// nonzero length; buildDecodeTable must accept the result.
	lens := buildLengthLimitedLengths(freqs[:], maxCodewordLen)
	codes := buildCanonicalCodes(lens)

	// Emit: header then bitstream.
	out := make([]byte, lensBytes, lensBytes+len(in)/2+64)
	for i := 0; i < lensBytes; i++ {
		out[i] = lens[2*i] | (lens[2*i+1] << 4)
	}

	// Worst-case bitstream size: an all-literal stream costs up to 15 bits per
	// byte; pad generously so the fixed-size writer never overflows.
	bufSize := 2*len(in) + 512
	bw := newBitWriter(bufSize)
	for _, tk := range tokens {
		bw.putBits(uint32(codes[tk.sym]), uint(lens[tk.sym]))
		if tk.sym >= numChars {
			// Match: emit length-overflow bytes FIRST, then the offset extra
			// bits — exactly the call order of wimlib's xpress_write_item
			// (the separate byte/bit pointers make this ordering load-bearing
			// whenever a bit flush rotates next_byte).
			log2Offset := uint((tk.sym >> 4) & 0xf)
			lengthCode := tk.sym & 0xf
			if lengthCode == 0xf {
				// Length overflow: matchLen-3 did not fit in 4 bits.
				L := tk.matchLen - minMatchLen // adjusted_len
				rem := L - 0xf
				if rem < 0xff {
					bw.putByte(byte(rem))
				} else {
					bw.putByte(0xff)
					bw.putU16(L)
				}
			}
			if log2Offset > 0 {
				extra := uint32(tk.offset - (1 << log2Offset))
				bw.putBits(extra, log2Offset)
			}
		}
	}
	out = append(out, bw.finish()...)
	return out, nil
}

// token is one parsed LZ77 element: a literal (sym 0-255) or a match
// (sym 256-511, with offset and matchLen for the extra bits / overflow bytes).
type token struct {
	sym      int
	offset   int
	matchLen int
}

// matchSym returns the XPRESS match-header symbol for a match.
func matchSym(offset, matchLen int) (sym, log2Offset, lengthCode int) {
	log2Offset = bits.Len(uint(offset)) - 1
	lengthCode = matchLen - minMatchLen
	if lengthCode > 0xf {
		lengthCode = 0xf
	}
	sym = numChars | (log2Offset << 4) | lengthCode
	return
}

// lz77Parse runs a hash-chain match finder with lazy matching and returns the
// token stream plus per-symbol frequencies.
func lz77Parse(in []byte) ([]token, [numSymbols]int) {
	var freqs [numSymbols]int
	n := len(in)
	tokens := make([]token, 0, n/2+1)

	head := make([]int, hashSize)
	for i := range head {
		head[i] = -1
	}
	prev := make([]int, n)

	hash3 := func(p int) uint32 {
		// 3-byte hash.
		v := uint32(in[p]) | uint32(in[p+1])<<8 | uint32(in[p+2])<<16
		return (v * 2654435761) >> (32 - hashBits)
	}

	insert := func(p int) {
		if p+2 >= n {
			return
		}
		h := hash3(p)
		prev[p] = head[h]
		head[h] = p
	}

	// findMatch searches the hash chain for the longest match ending at most at
	// the end of input, with offset in [1, p].
	findMatch := func(p int) (bestLen, bestOff int) {
		if p+minMatchLen > n {
			return 0, 0
		}
		maxLen := n - p
		if maxLen > maxMatchLen {
			maxLen = maxMatchLen
		}
		h := hash3(p)
		cur := head[h]
		chain := maxChainLen
		bestLen = minMatchLen - 1
		for cur >= 0 && chain > 0 {
			chain--
			// Quick reject: compare the byte just past the current best.
			if bestLen < maxLen && in[cur+bestLen] == in[p+bestLen] {
				l := 0
				for l < maxLen && in[cur+l] == in[p+l] {
					l++
				}
				if l > bestLen {
					bestLen = l
					bestOff = p - cur
					if l >= niceMatch || l >= maxLen {
						break
					}
				}
			}
			cur = prev[cur]
		}
		if bestLen < minMatchLen {
			return 0, 0
		}
		return bestLen, bestOff
	}

	emitLiteral := func(b byte) {
		tokens = append(tokens, token{sym: int(b)})
		freqs[int(b)]++
	}
	emitMatch := func(offset, matchLen int) {
		sym, _, _ := matchSym(offset, matchLen)
		tokens = append(tokens, token{sym: sym, offset: offset, matchLen: matchLen})
		freqs[sym]++
	}

	// insertRange inserts every position in [lo, hi) into the hash chains,
	// skipping any already inserted (tracked by the caller via inserted).
	insertRange := func(lo, hi, inserted int) {
		for p := lo; p < hi; p++ {
			if p < n && p != inserted {
				insert(p)
			}
		}
	}

	i := 0
	for i < n {
		curLen, curOff := findMatch(i)
		if curLen >= minMatchLen {
			// Lazy matching: if a longer match starts at i+1, emit i as a
			// literal and take that match instead.
			if curLen < niceMatch && i+1 < n {
				insert(i) // mark i as inserted (we look ahead from i+1)
				nextLen, nextOff := findMatch(i + 1)
				if nextLen > curLen {
					emitLiteral(in[i])
					i++
					emitMatch(nextOff, nextLen)
					insertRange(i, i+nextLen, i-1) // i-1 already inserted
					i += nextLen
					continue
				}
				emitMatch(curOff, curLen)
				insertRange(i, i+curLen, i) // i already inserted
				i += curLen
				continue
			}
			emitMatch(curOff, curLen)
			insertRange(i, i+curLen, -1)
			i += curLen
			continue
		}
		insert(i)
		emitLiteral(in[i])
		i++
	}
	return tokens, freqs
}

// buildLengthLimitedLengths assigns canonical Huffman code lengths (1..maxLen)
// to every symbol with a nonzero frequency, guaranteeing a complete or
// under-subscribed code that buildDecodeTable accepts. Symbols with zero
// frequency get length 0.
func buildLengthLimitedLengths(freqs []int, maxLen int) [numSymbols]byte {
	var out [numSymbols]byte

	// Collect used symbols.
	used := make([]symFreq, 0, numSymbols)
	for s, f := range freqs {
		if f > 0 {
			used = append(used, symFreq{s, f})
		}
	}

	switch len(used) {
	case 0:
		return out
	case 1:
		// A single symbol needs at least one bit so the bitstream is non-empty
		// and decodable.
		out[used[0].sym] = 1
		return out
	}

	// Build an optimal Huffman tree, then enforce the length limit via the
	// classic "build then limit + Kraft repair" approach.
	lens := huffmanCodeLengths(used)
	limitLengths(lens, used, maxLen)

	for s, l := range lens {
		out[s] = byte(l)
	}
	return out
}

// symFreq pairs a symbol with its frequency.
type symFreq struct {
	sym  int
	freq int
}

// huffmanCodeLengths computes optimal (unbounded) Huffman code lengths for the
// used symbols, indexed by symbol number across the full alphabet.
func huffmanCodeLengths(used []symFreq) []int {
	lens := make([]int, numSymbols)

	type node struct {
		freq        int
		left, right int // child indices into nodes, -1 for leaf
		sym         int // symbol for leaves, -1 otherwise
	}
	nodes := make([]node, 0, 2*len(used))
	// Leaves.
	for _, u := range used {
		nodes = append(nodes, node{freq: u.freq, left: -1, right: -1, sym: u.sym})
	}

	// Min-heap over node indices by frequency (ties: lower index first for
	// determinism).
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
	// Simple build/sort-based priority queue (alphabets are tiny: <=512).
	sort.Slice(heap, func(i, j int) bool { return less(heap[i], heap[j]) })

	pop := func() int {
		v := heap[0]
		heap = heap[1:]
		return v
	}
	push := func(idx int) {
		// Insert keeping sorted order.
		pos := sort.Search(len(heap), func(i int) bool { return !less(heap[i], idx) })
		heap = append(heap, 0)
		copy(heap[pos+1:], heap[pos:])
		heap[pos] = idx
	}

	for len(heap) > 1 {
		a := pop()
		b := pop()
		nodes = append(nodes, node{freq: nodes[a].freq + nodes[b].freq, left: a, right: b, sym: -1})
		push(len(nodes) - 1)
	}
	root := heap[0]

	// Walk to assign depths (= code lengths).
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
	walk(root, 0)
	return lens
}

// limitLengths enforces a maximum code length while keeping the Kraft sum <= 1
// (so buildDecodeTable accepts it). It clamps over-long codes to maxLen, then
// repairs the resulting over-subscription by lengthening short codes.
func limitLengths(lens []int, used []symFreq, maxLen int) {
	// Clamp.
	for _, u := range used {
		if lens[u.sym] > maxLen {
			lens[u.sym] = maxLen
		}
	}

	// Compute Kraft sum scaled to 2^maxLen.
	one := 1 << maxLen
	kraft := 0
	for _, u := range used {
		kraft += one >> lens[u.sym]
	}

	// If over-subscribed (kraft > one), lengthen shallow codes (smallest length
	// first reduces Kraft most per step) until within budget.
	if kraft > one {
		// Sort symbols by current length ascending (then freq ascending so we
		// prefer demoting rarer symbols among equal lengths).
		order := make([]int, len(used))
		for i := range used {
			order[i] = used[i].sym
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
				break // all at maxLen; cannot reduce further (shouldn't happen)
			}
			// Re-sort since lengths changed.
			sort.Slice(order, func(i, j int) bool {
				if lens[order[i]] != lens[order[j]] {
					return lens[order[i]] < lens[order[j]]
				}
				return order[i] < order[j]
			})
		}
	}

	// Optionally tighten: if under-subscribed we could shorten codes to improve
	// ratio, but correctness only needs Kraft sum <= 1, which holds. Leave as is.
}

// buildCanonicalCodes assigns canonical Huffman codewords matching the decoder's
// assignment order in buildDecodeTable: symbols ordered by (length asc, symbol
// asc), codes assigned sequentially and shifted left between lengths. The
// returned code value is right-justified (LSB = last bit), ready for MSB-first
// emission of `length` bits.
func buildCanonicalCodes(lens [numSymbols]byte) [numSymbols]uint32 {
	var codes [numSymbols]uint32
	code := 0
	for length := 1; length <= maxCodewordLen; length++ {
		for sym := 0; sym < numSymbols; sym++ {
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

// bitWriter is a direct port of wimlib's xpress_output_bitstream. It writes to
// a fixed-size buffer using three pointers: nextBits / nextBits2 point at the
// two reserved 16-bit word slots that pending bits will fill, and nextByte
// points at where the next raw (length-overflow) byte goes. Bits accumulate
// MSB-first into bitbuf and flush as 16-bit little-endian words; raw bytes
// interleave at nextByte. This pointer pipeline reproduces exactly the byte
// order the decoder's shared word/byte cursor consumes.
type bitWriter struct {
	buf       []byte
	bitbuf    uint32
	bitcount  uint
	nextBits  int
	nextBits2 int
	nextByte  int
}

func newBitWriter(size int) *bitWriter {
	if size < 4 {
		size = 4
	}
	return &bitWriter{
		buf:       make([]byte, size),
		nextBits:  0,
		nextBits2: 2,
		nextByte:  4,
	}
}

func (w *bitWriter) putLE16(at int, v uint16) {
	w.buf[at] = byte(v)
	w.buf[at+1] = byte(v >> 8)
}

// putBits adds num bits (num <= 16) MSB-first, mirroring xpress_write_bits.
func (w *bitWriter) putBits(value uint32, num uint) {
	w.bitcount += num
	w.bitbuf = (w.bitbuf << num) | (value & ((1 << num) - 1))
	if w.bitcount > 16 {
		w.bitcount -= 16
		if len(w.buf)-w.nextByte >= 2 {
			w.putLE16(w.nextBits, uint16(w.bitbuf>>w.bitcount))
			w.nextBits = w.nextBits2
			w.nextBits2 = w.nextByte
			w.nextByte += 2
		}
	}
}

// putByte interweaves a raw byte, mirroring xpress_write_byte.
func (w *bitWriter) putByte(b byte) {
	if w.nextByte < len(w.buf) {
		w.buf[w.nextByte] = b
		w.nextByte++
	}
}

// putU16 interweaves a raw little-endian 16-bit value, mirroring
// xpress_write_u16.
func (w *bitWriter) putU16(v int) {
	if len(w.buf)-w.nextByte >= 2 {
		w.putLE16(w.nextByte, uint16(v))
		w.nextByte += 2
	}
}

// finish flushes the final coding unit (xpress_flush_output) and returns the
// bytes actually written.
func (w *bitWriter) finish() []byte {
	if len(w.buf)-w.nextByte < 2 {
		return w.buf // overflow; caller-sized buffer should prevent this
	}
	w.putLE16(w.nextBits, uint16(w.bitbuf<<(16-w.bitcount)))
	w.putLE16(w.nextBits2, 0)
	return w.buf[:w.nextByte]
}
