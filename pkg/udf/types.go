package udf

import (
	"encoding/binary"
	"time"
	"unicode/utf16"
)

// encodeDString encodes s as an OSTA CS0 d-string occupying exactly fieldLen
// bytes: compressed-unicode dchars followed, in the final byte, by the number
// of used bytes.
func encodeDString(s string, fieldLen int) []byte {
	buf := make([]byte, fieldLen)
	if s == "" {
		return buf
	}
	// Use 8-bit compression when every rune fits in a byte, else UTF-16BE.
	eightBit := true
	for _, r := range s {
		if r > 0xFF {
			eightBit = false
			break
		}
	}

	var used int
	if eightBit {
		buf[0] = 8
		used = 1
		for _, r := range s {
			if used >= fieldLen-1 {
				break
			}
			buf[used] = byte(r)
			used++
		}
	} else {
		buf[0] = 16
		used = 1
		for _, u := range utf16.Encode([]rune(s)) {
			if used+2 > fieldLen-1 {
				break
			}
			binary.BigEndian.PutUint16(buf[used:], u)
			used += 2
		}
	}
	buf[fieldLen-1] = byte(used)
	return buf
}

// encodeTimestamp encodes t as a 12-byte ECMA-167 timestamp (UTC, type "local").
func encodeTimestamp(t time.Time) []byte {
	t = t.UTC()
	b := make([]byte, 12)
	// Type 1 (local time) in the high nibble; timezone 0.
	binary.LittleEndian.PutUint16(b[0:], 1<<12)
	binary.LittleEndian.PutUint16(b[2:], uint16(t.Year()))
	b[4] = byte(t.Month())
	b[5] = byte(t.Day())
	b[6] = byte(t.Hour())
	b[7] = byte(t.Minute())
	b[8] = byte(t.Second())
	return b
}

// charSpec returns the 64-byte CharacterSet field: type 0 (CS0) plus the
// "OSTA Compressed Unicode" identifier.
func charSpec() []byte {
	b := make([]byte, 64)
	b[0] = 0 // CS0
	copy(b[1:], "OSTA Compressed Unicode")
	return b
}

// entityID builds a 32-byte EntityID: 1 flag byte, a 23-byte identifier, and an
// 8-byte suffix.
func entityID(id string, suffix []byte) []byte {
	b := make([]byte, 32)
	copy(b[1:24], id)
	copy(b[24:32], suffix)
	return b
}

// domainEntityID is the "*OSTA UDF Compliant" domain identifier with a UDF 1.02
// domain suffix (revision 0x0102, no domain flags).
func domainEntityID() []byte {
	suffix := make([]byte, 8)
	binary.LittleEndian.PutUint16(suffix[0:], 0x0102) // UDF revision
	return entityID("*OSTA UDF Compliant", suffix)
}

// implEntityID is this writer's implementation identifier, with a UDF
// implementation suffix (OS class/identifier left unspecified).
func implEntityID() []byte {
	return entityID("*winmediafoundry", make([]byte, 8))
}

func utf16Encode(s string) []uint16 { return utf16.Encode([]rune(s)) }

func utf16Count(s string) int { return len(utf16.Encode([]rune(s))) }

// shortAD writes an 8-byte short allocation descriptor (length, logical block).
func shortAD(length, location uint32) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint32(b[0:], length)
	binary.LittleEndian.PutUint32(b[4:], location)
	return b
}

// longAD writes a 16-byte long allocation descriptor (length, block, partition).
func longAD(length uint32, location uint32, partition uint16) []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint32(b[0:], length)
	binary.LittleEndian.PutUint32(b[4:], location)
	binary.LittleEndian.PutUint16(b[8:], partition)
	return b
}
