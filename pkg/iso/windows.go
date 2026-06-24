package iso

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Standard Windows media boot images, relative to the media root. The casing
// Microsoft uses on the EFI path varies; lookup is case-insensitive.
const (
	winBIOSBoot = "boot/etfsboot.com"
	winUEFIBoot = "efi/microsoft/boot/efisys.bin"
	// winUEFIBootARM64 is the ARM64 UEFI boot loader on the media root. Its
	// presence marks the media as ARM64 (UEFI-only); x64 media carries
	// efi/boot/bootx64.efi instead. Lookup keys are lower-cased (see indexTree).
	winUEFIBootARM64 = "efi/boot/bootaa64.efi"
	// biosBootLoadSize matches oscdimg's "-boot-load-size 8" for etfsboot.com.
	biosBootLoadSize = 8
)

// WindowsBootEntries inspects an extracted media root and returns the El Torito
// entries for whichever Windows boot images are present (BIOS via
// boot/etfsboot.com, UEFI via efi/microsoft/boot/efisys.bin). It errors if
// neither is found.
func WindowsBootEntries(mediaRoot string) ([]BootEntry, error) {
	index, err := indexTree(mediaRoot)
	if err != nil {
		return nil, err
	}

	var entries []BootEntry
	if rel, ok := index[winBIOSBoot]; ok {
		entries = append(entries, BootEntry{Firmware: FirmwareBIOS, BootFile: rel, LoadSize: biosBootLoadSize})
	}
	if rel, ok := index[winUEFIBoot]; ok {
		entries = append(entries, BootEntry{Firmware: FirmwareUEFI, BootFile: rel})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("iso: no Windows boot images found under %s (expected %s and/or %s)",
			mediaRoot, winBIOSBoot, winUEFIBoot)
	}
	return entries, nil
}

// BuildWindowsISO masters a bootable Windows installation ISO from an extracted
// media root (typically the applied "Windows Setup Media" image plus
// sources/boot.wim and sources/install.wim or .esd).
func BuildWindowsISO(mediaRoot, outPath, volumeID string) error {
	entries, err := WindowsBootEntries(mediaRoot)
	if err != nil {
		return err
	}
	return Build(mediaRoot, outPath, Options{
		VolumeID:    volumeID,
		Publisher:   "MICROSOFT CORPORATION",
		BootEntries: entries,
	})
}

// indexTree maps lowercased slash-relative paths to their actual on-disk
// relative path, so boot images can be found regardless of case.
func indexTree(root string) (map[string]string, error) {
	index := make(map[string]string)
	err := filepath.WalkDir(root, func(p string, de os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if de.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return e
		}
		rel = filepath.ToSlash(rel)
		index[strings.ToLower(rel)] = rel
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("iso: index %s: %w", root, err)
	}
	return index, nil
}
