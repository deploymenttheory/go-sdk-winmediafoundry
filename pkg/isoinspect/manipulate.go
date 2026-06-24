package isoinspect

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// ExtractElToritoEFIImage writes the FAT EFI System Partition image referenced
// by the El Torito UEFI boot entry (efisys.bin's contents) of the ISO at isoPath
// to destPath. It is the volume the firmware mounts to launch \EFI\BOOT\
// BOOTAA64.EFI, useful for inspecting the boot image in isolation.
func ExtractElToritoEFIImage(isoPath, destPath string) error {
	rep, err := Inspect(isoPath)
	if err != nil {
		return err
	}
	if rep.ElTorito == nil || rep.ElTorito.UEFI == nil {
		return fmt.Errorf("isoinspect: %s has no El Torito UEFI boot entry", isoPath)
	}
	uefi := rep.ElTorito.UEFI

	src, err := os.Open(isoPath) //nolint:gosec // caller-provided path
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(destPath) //nolint:gosec // caller-provided path
	if err != nil {
		return err
	}
	defer dst.Close()

	off := int64(uefi.LoadRBA) * sectorSize
	n := uefi.SizeBytes()
	if _, err := io.Copy(dst, io.NewSectionReader(src, off, n)); err != nil {
		return fmt.Errorf("isoinspect: extract EFI image: %w", err)
	}
	return nil
}

// SetElToritoUEFIOnly rewrites the El Torito boot catalog of the ISO at isoPath
// in place so the UEFI boot image becomes the validation/default entry with the
// validation platform set to UEFI (0xEF), dropping any BIOS (x86) entry. This
// matches the layout of Microsoft's UEFI-only ARM64 media and removes the
// "Image type X64 can't be loaded" firmware error caused by a stray etfsboot.com
// BIOS entry. The image's UDF/El Torito boot image data are untouched.
//
// It is a no-op (returning nil) when the catalog is already UEFI-only with the
// UEFI image as the default entry.
func SetElToritoUEFIOnly(isoPath string) error {
	rep, err := Inspect(isoPath)
	if err != nil {
		return err
	}
	et := rep.ElTorito
	if et == nil || !et.Present {
		return fmt.Errorf("isoinspect: %s has no El Torito boot catalog", isoPath)
	}
	if et.UEFI == nil {
		return fmt.Errorf("isoinspect: %s has no UEFI boot entry to promote", isoPath)
	}
	if et.ValidationPlatform == platformUEFI && et.BIOS == nil && !et.UEFI.Section {
		return nil // already UEFI-only default
	}

	cat := make([]byte, sectorSize)
	le := binary.LittleEndian

	// Validation entry: header 0x01, platform UEFI, 55AA key, zero-sum checksum.
	cat[0] = 0x01
	cat[1] = platformUEFI
	cat[30] = 0x55
	cat[31] = 0xAA
	var sum uint16
	for i := 0; i < 32; i += 2 {
		if i == 28 {
			continue
		}
		sum += le.Uint16(cat[i:])
	}
	le.PutUint16(cat[28:], -sum)

	// Default/initial entry: the UEFI boot image, no emulation.
	cat[32] = 0x88
	cat[33] = 0x00
	le.PutUint16(cat[32+6:], et.UEFI.SectorCount)
	le.PutUint32(cat[32+8:], et.UEFI.LoadRBA)

	f, err := os.OpenFile(isoPath, os.O_RDWR, 0) //nolint:gosec // caller-provided path
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteAt(cat, int64(et.CatalogSector)*sectorSize); err != nil {
		return fmt.Errorf("isoinspect: rewrite boot catalog: %w", err)
	}
	return nil
}
