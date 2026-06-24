package isoinspect

import (
	"encoding/binary"
	"fmt"
)

// El Torito platform identifiers.
const (
	platformBIOS = 0x00
	platformUEFI = 0xEF
)

// BootEntry is one El Torito boot-catalog entry (the default/initial entry or a
// section entry).
type BootEntry struct {
	// Section is true for an entry introduced by a section header; false for the
	// initial/default entry.
	Section bool
	// PlatformID is the entry's platform: 0x00 BIOS, 0xEF UEFI. For the default
	// entry this is the validation entry's platform.
	PlatformID uint8
	Bootable   bool
	MediaType  uint8 // 0 = no emulation
	// SectorCount is the El Torito "sector count": the number of 512-byte virtual
	// sectors of the boot image. For a UEFI entry this is the size of the FAT EFI
	// System Partition image the firmware exposes.
	SectorCount uint16
	// LoadRBA is the logical block (2048-byte) where the boot image begins.
	LoadRBA uint32
	// ImageIsFAT/ImageIsX86Boot describe the bytes at LoadRBA.
	ImageIsFAT     bool
	ImageIsX86Boot bool
}

// SizeBytes returns the boot image size implied by SectorCount (512-byte units).
func (e BootEntry) SizeBytes() int64 { return int64(e.SectorCount) * 512 }

// ElToritoInfo describes a parsed El Torito boot catalog.
type ElToritoInfo struct {
	// Present is false when the image has no El Torito boot record.
	Present              bool
	CatalogSector        uint32
	ValidationPlatform   uint8
	ValidationChecksumOK bool
	Entries              []BootEntry
	// UEFI and BIOS point to the resolved boot entries of each kind, if present.
	UEFI *BootEntry
	BIOS *BootEntry
}

func inspectElTorito(v *volume, r *Report) *ElToritoInfo {
	info := &ElToritoInfo{}

	// Find the El Torito Boot Record Volume Descriptor (type 0, "EL TORITO
	// SPECIFICATION") in the volume descriptor area (sectors 16..32).
	var catSector uint32
	found := false
	for s := uint64(16); s < 33; s++ {
		d, err := v.sector(s)
		if err != nil {
			break
		}
		if d[0] == 0 && string(d[1:6]) == "CD001" && string(d[7:30]) == "EL TORITO SPECIFICATION" {
			catSector = binary.LittleEndian.Uint32(d[71:])
			found = true
			break
		}
	}
	if !found {
		r.addError("eltorito", "no El Torito boot record found — image is not UEFI-bootable")
		return info
	}
	info.Present = true
	info.CatalogSector = catSector

	cat, err := v.sector(uint64(catSector))
	if err != nil {
		r.addError("eltorito", fmt.Sprintf("cannot read boot catalog at sector %d: %v", catSector, err))
		return info
	}

	// Validation entry (bytes 0..31).
	info.ValidationPlatform = cat[1]
	if cat[0] != 0x01 || cat[30] != 0x55 || cat[31] != 0xAA {
		r.addError("eltorito", "boot catalog validation entry is malformed (bad header or 55AA key)")
	}
	var sum uint16
	for i := 0; i < 32; i += 2 {
		sum += binary.LittleEndian.Uint16(cat[i:])
	}
	info.ValidationChecksumOK = sum == 0
	if !info.ValidationChecksumOK {
		r.addError("eltorito", fmt.Sprintf("boot catalog validation checksum is non-zero (%#04x)", sum))
	}

	// Default/initial entry (bytes 32..63); its platform is the validation entry's.
	def := parseBootEntry(v, cat[32:64], false, info.ValidationPlatform)
	info.Entries = append(info.Entries, def)

	// Section headers + entries (bytes 64+).
	off := 64
	for off+64 <= len(cat) && (cat[off] == 0x90 || cat[off] == 0x91) {
		plat := cat[off+1]
		nEntries := int(binary.LittleEndian.Uint16(cat[off+2:]))
		if nEntries == 0 {
			nEntries = 1
		}
		entryOff := off + 32
		for k := 0; k < nEntries && entryOff+32 <= len(cat); k++ {
			info.Entries = append(info.Entries, parseBootEntry(v, cat[entryOff:entryOff+32], true, plat))
			entryOff += 32
		}
		off = entryOff
	}

	classifyAndValidate(info, r)
	return info
}

// parseBootEntry decodes a 32-byte boot entry and inspects the bytes at its
// load address.
func parseBootEntry(v *volume, e []byte, section bool, platform uint8) BootEntry {
	be := BootEntry{
		Section:     section,
		PlatformID:  platform,
		Bootable:    e[0] == 0x88,
		MediaType:   e[1],
		SectorCount: binary.LittleEndian.Uint16(e[6:]),
		LoadRBA:     binary.LittleEndian.Uint32(e[8:]),
	}
	if head, err := v.sector(uint64(be.LoadRBA)); err == nil {
		// FAT boot sector: jump (EB..90 / E9) + 55AA signature at 510.
		if head[510] == 0x55 && head[511] == 0xAA && (head[0] == 0xEB || head[0] == 0xE9) {
			be.ImageIsFAT = true
		}
		// Legacy x86 boot code commonly starts with CLI (0xFA) — etfsboot.com.
		if head[0] == 0xFA && !be.ImageIsFAT {
			be.ImageIsX86Boot = true
		}
	}
	return be
}

// classifyAndValidate fills UEFI/BIOS pointers and emits El Torito issues.
func classifyAndValidate(info *ElToritoInfo, r *Report) {
	for i := range info.Entries {
		e := &info.Entries[i]
		switch e.PlatformID {
		case platformUEFI:
			if info.UEFI == nil {
				info.UEFI = e
			}
		case platformBIOS:
			if info.BIOS == nil {
				info.BIOS = e
			}
		}
	}

	if info.UEFI == nil {
		r.addError("eltorito", "no UEFI (platform 0xEF) boot entry — ARM64/UEFI media will not boot")
		return
	}
	if !info.UEFI.ImageIsFAT {
		r.addError("eltorito", fmt.Sprintf("UEFI boot image at sector %d is not a FAT EFI System Partition", info.UEFI.LoadRBA))
	}
	if info.UEFI.SectorCount < 2 {
		// 0/1 means "to end of media" — valid but unusual for a fixed efisys.bin.
		r.addWarning("eltorito", "UEFI entry sector count is 0/1 (extends to end of media)")
	}

	// A BIOS (x86) entry on UEFI-only ARM64 media is unnecessary and makes the
	// firmware log "Image type X64 can't be loaded on AARCH64 UEFI system".
	if info.BIOS != nil && info.BIOS.ImageIsX86Boot {
		r.addWarning("eltorito",
			"a BIOS (x86 etfsboot.com) boot entry is present; Microsoft's ARM64 media is UEFI-only — "+
				"the firmware will log an 'Image type X64 can't be loaded' error trying to load it")
	}

	// Microsoft's ARM64 media makes the UEFI image the validation/default entry
	// (validation platform 0xEF). A BIOS-default + UEFI-section layout (validation
	// platform 0x00) is the x64 dual-boot shape and atypical for ARM64.
	if info.ValidationPlatform != platformUEFI && info.UEFI.Section {
		r.addWarning("eltorito",
			"validation platform is BIOS (0x00) with the UEFI image in a section; "+
				"Microsoft ARM64 media uses a UEFI-platform (0xEF) default entry")
	}
}
