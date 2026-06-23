// Package iso assembles bootable ISO9660 images. It is built on go-diskfs and
// adds the dual BIOS+UEFI El Torito boot catalog that Windows installation media
// uses, plus helpers that recognise the boot images laid down by an extracted
// "Windows Setup Media" image.
//
// Build masters a plain ISO9660+Joliet image, whose single-file size is capped
// at 4 GiB by ISO9660. BuildWindowsUDF masters a UDF + El Torito "bridge" image
// (using the sibling udf package) that removes that limit, so a full
// install.wim/install.esd larger than 4 GiB fits — this is the format real
// Windows media uses.
package iso

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/filesystem/iso9660"
	"github.com/diskfs/go-diskfs/partition/mbr"
)

// Firmware identifies the platform an El Torito boot entry targets.
type Firmware int

const (
	// FirmwareBIOS is a legacy PC BIOS boot entry (El Torito platform 0x00).
	FirmwareBIOS Firmware = iota
	// FirmwareUEFI is a UEFI boot entry (El Torito platform 0xEF).
	FirmwareUEFI
)

// BootEntry describes one no-emulation El Torito boot entry.
type BootEntry struct {
	Firmware Firmware
	// BootFile is the slash-separated path of the boot image within the media
	// root (e.g. "boot/etfsboot.com").
	BootFile string
	// LoadSize is the boot load size in virtual (512-byte) sectors. Zero lets
	// the library choose (file size for UEFI, a small default for BIOS).
	LoadSize uint16
}

// Options configures an ISO build.
type Options struct {
	VolumeID    string
	Publisher   string
	BootEntries []BootEntry
}

const isoBlockSize = 2048

// Build masters the directory tree at srcDir into an ISO9660+Joliet image at
// outPath, with the El Torito boot entries from opts.
func Build(srcDir, outPath string, opts Options) error {
	dirs, files, totalBytes, err := scanTree(srcDir)
	if err != nil {
		return err
	}

	// Size the backing file generously: payload plus headroom for the volume
	// descriptors, path tables, directory records (also duplicated for Joliet),
	// and the boot catalog. 16 MiB + 20% comfortably covers the metadata.
	size := totalBytes + totalBytes/5 + 16<<20
	size = (size + isoBlockSize - 1) / isoBlockSize * isoBlockSize

	d, err := diskfs.Create(outPath, size, diskfs.SectorSize(isoBlockSize))
	if err != nil {
		return fmt.Errorf("iso: create image: %w", err)
	}
	defer d.Close()

	fsi, err := d.CreateFilesystem(disk.FilesystemSpec{Partition: 0, FSType: filesystem.TypeISO9660})
	if err != nil {
		return fmt.Errorf("iso: create filesystem: %w", err)
	}

	if err := writeTree(fsi, srcDir, dirs, files); err != nil {
		return err
	}

	isofs, ok := fsi.(*iso9660.FileSystem)
	if !ok {
		return fmt.Errorf("iso: unexpected filesystem type %T", fsi)
	}

	final := iso9660.FinalizeOptions{
		Joliet:              true,
		VolumeIdentifier:    opts.VolumeID,
		PublisherIdentifier: opts.Publisher,
	}
	if et := buildElTorito(opts.BootEntries); et != nil {
		final.ElTorito = et
	}
	if err := isofs.Finalize(final); err != nil {
		return fmt.Errorf("iso: finalize: %w", err)
	}
	return nil
}

func buildElTorito(entries []BootEntry) *iso9660.ElTorito {
	if len(entries) == 0 {
		return nil
	}
	et := &iso9660.ElTorito{
		BootCatalog:     "/boot.catalog",
		HideBootCatalog: true,
	}
	for _, e := range entries {
		platform := iso9660.BIOS
		if e.Firmware == FirmwareUEFI {
			platform = iso9660.EFI
		}
		entry := &iso9660.ElToritoEntry{
			Platform:  platform,
			Emulation: iso9660.NoEmulation,
			BootFile:  "/" + strings.TrimPrefix(filepath.ToSlash(e.BootFile), "/"),
			// Windows media keeps the boot images visible in the listing.
			HideBootFile: false,
			SystemType:   mbr.Fat32LBA,
		}
		if e.LoadSize != 0 {
			entry.SetLoadSize(e.LoadSize)
		}
		et.Entries = append(et.Entries, entry)
	}
	return et
}

// scanTree returns the directories (relative, slash paths) and files under
// root, plus the total byte size of all files.
func scanTree(root string) (dirs, files []string, totalBytes int64, err error) {
	err = filepath.WalkDir(root, func(p string, de os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return e
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if de.IsDir() {
			dirs = append(dirs, rel)
			return nil
		}
		info, e := de.Info()
		if e != nil {
			return e
		}
		files = append(files, rel)
		totalBytes += info.Size()
		return nil
	})
	if err != nil {
		return nil, nil, 0, fmt.Errorf("iso: scan %s: %w", root, err)
	}
	return dirs, files, totalBytes, nil
}

func writeTree(fsi filesystem.FileSystem, srcDir string, dirs, files []string) error {
	// Create directories shallowest-first so parents exist before children.
	sort.Slice(dirs, func(i, j int) bool {
		return strings.Count(dirs[i], "/") < strings.Count(dirs[j], "/")
	})
	for _, d := range dirs {
		if err := fsi.Mkdir("/" + d); err != nil {
			return fmt.Errorf("iso: mkdir %s: %w", d, err)
		}
	}
	for _, f := range files {
		if err := copyInto(fsi, filepath.Join(srcDir, filepath.FromSlash(f)), "/"+f); err != nil {
			return err
		}
	}
	return nil
}

func copyInto(fsi filesystem.FileSystem, srcPath, isoPath string) error {
	src, err := os.Open(srcPath) //nolint:gosec // caller-provided media tree
	if err != nil {
		return fmt.Errorf("iso: open %s: %w", srcPath, err)
	}
	defer src.Close()

	if dir := path.Dir(isoPath); dir != "/" && dir != "." {
		_ = fsi.Mkdir(dir) // ensure parent exists (idempotent)
	}
	dst, err := fsi.OpenFile(isoPath, os.O_CREATE|os.O_RDWR)
	if err != nil {
		return fmt.Errorf("iso: create %s in image: %w", isoPath, err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("iso: write %s: %w", isoPath, err)
	}
	return nil
}
