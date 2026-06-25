// Throwaway: dump a WIM's header boot-relevant fields + XML. usage: wimhdr <wim>
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"unicode/utf16"
)

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer f.Close()
	hdr := make([]byte, 208)
	if _, err := f.ReadAt(hdr, 0); err != nil {
		fmt.Println("read header:", err)
		os.Exit(1)
	}
	le := binary.LittleEndian
	res := func(off int) (comp uint64, flags byte, o, orig uint64) {
		v := le.Uint64(hdr[off:])
		return v & 0x00FFFFFFFFFFFFFF, byte(v >> 56), le.Uint64(hdr[off+8:]), le.Uint64(hdr[off+16:])
	}
	fmt.Printf("file:        %s\n", os.Args[1])
	fmt.Printf("magic:       %q\n", string(hdr[:8]))
	fmt.Printf("version:     %#08x\n", le.Uint32(hdr[12:]))
	fmt.Printf("flags:       %#08x\n", le.Uint32(hdr[16:]))
	fmt.Printf("chunkSize:   %d\n", le.Uint32(hdr[20:]))
	fmt.Printf("imageCount:  %d\n", le.Uint32(hdr[44:]))
	bc, bf, bo, borig := res(96)
	fmt.Printf("bootMeta:    compSize=%d flags=%#x offset=%d origSize=%d\n", bc, bf, bo, borig)
	fmt.Printf("bootIndex:   %d\n", le.Uint32(hdr[120:]))
	xc, _, xo, _ := res(72)
	xb := make([]byte, xc)
	f.ReadAt(xb, int64(xo))
	if len(xb) >= 2 {
		xb = xb[2:] // skip UTF-16LE BOM
	}
	u16 := make([]uint16, len(xb)/2)
	for i := range u16 {
		u16[i] = le.Uint16(xb[i*2:])
	}
	fmt.Printf("XML:         %s\n", string(utf16.Decode(u16)))
}
