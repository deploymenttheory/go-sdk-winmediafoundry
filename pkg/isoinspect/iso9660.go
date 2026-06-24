package isoinspect

import (
	"encoding/binary"
	"strings"
)

// ISO9660Info describes the ISO9660 primary volume descriptor.
type ISO9660Info struct {
	Present      bool
	VolumeID     string
	VolumeBlocks uint32
	RootEntries  []string
	HasJoliet    bool
}

func inspectISO9660(v *volume, r *Report) *ISO9660Info {
	info := &ISO9660Info{}
	pvd, err := v.sector(16)
	if err != nil || pvd[0] != 1 || string(pvd[1:6]) != "CD001" {
		r.addError("iso9660", "no ISO9660 primary volume descriptor at sector 16")
		return info
	}
	info.Present = true
	info.VolumeID = strings.TrimSpace(string(pvd[40:72]))
	info.VolumeBlocks = binary.LittleEndian.Uint32(pvd[80:])

	// Supplementary volume descriptor (type 2) at sector 17+ indicates Joliet.
	for s := uint64(17); s < 24; s++ {
		d, err := v.sector(s)
		if err != nil {
			break
		}
		if d[0] == 255 && string(d[1:6]) == "CD001" {
			break
		}
		if d[0] == 2 && string(d[1:6]) == "CD001" {
			info.HasJoliet = true
		}
	}

	// Root directory listing (informational — Windows ARM64 media keeps the
	// payload in UDF, so an almost-empty ISO9660 root is normal, not a defect).
	rootRec := pvd[156 : 156+34]
	ext := binary.LittleEndian.Uint32(rootRec[2:])
	length := binary.LittleEndian.Uint32(rootRec[10:])
	info.RootEntries = listISODir(v, ext, length)
	return info
}

func listISODir(v *volume, extentSector, length uint32) []string {
	if length == 0 {
		length = sectorSize
	}
	data, err := v.read(int64(extentSector)*sectorSize, int(length))
	if err != nil {
		return nil
	}
	var names []string
	for off := 0; off < len(data); {
		recLen := int(data[off])
		if recLen == 0 {
			// Advance to the next logical block.
			next := (off/sectorSize + 1) * sectorSize
			if next <= off || next >= len(data) {
				break
			}
			off = next
			continue
		}
		if off+33 > len(data) {
			break
		}
		nlen := int(data[off+32])
		name := data[off+33 : off+33+nlen]
		switch {
		case nlen == 1 && name[0] == 0x00:
			names = append(names, ".")
		case nlen == 1 && name[0] == 0x01:
			names = append(names, "..")
		default:
			n := string(name)
			if i := strings.IndexByte(n, ';'); i >= 0 {
				n = n[:i]
			}
			names = append(names, n)
		}
		off += recLen
	}
	return names
}
