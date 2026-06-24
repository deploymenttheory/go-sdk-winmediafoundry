package isoinspect

import (
	"fmt"
	"sort"
	"strings"
)

// Summary returns a human-readable multi-line summary of the report: the parsed
// ISO9660 / El Torito / UDF structure followed by every issue.
func (r *Report) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "ISO: %s (%.2f GB)\n", r.Path, float64(r.Size)/1e9)

	if i := r.ISO9660; i != nil && i.Present {
		fmt.Fprintf(&b, "ISO9660: volume %q, %d blocks, joliet=%v, root entries=%v\n",
			i.VolumeID, i.VolumeBlocks, i.HasJoliet, i.RootEntries)
	}

	if e := r.ElTorito; e != nil && e.Present {
		fmt.Fprintf(&b, "El Torito: catalog@%d validation-platform=%#02x checksumOK=%v\n",
			e.CatalogSector, e.ValidationPlatform, e.ValidationChecksumOK)
		for _, en := range e.Entries {
			kind := "default"
			if en.Section {
				kind = "section"
			}
			fmt.Fprintf(&b, "  - %s platform=%#02x bootable=%v fat=%v x86=%v sectorCount=%d (%d bytes) loadRBA=%d\n",
				kind, en.PlatformID, en.Bootable, en.ImageIsFAT, en.ImageIsX86Boot,
				en.SectorCount, en.SizeBytes(), en.LoadRBA)
		}
	}

	if u := r.UDF; u != nil && u.Present {
		fmt.Fprintf(&b, "UDF: rev=%#04x volume=%q partStart=%d partBlocks=%d files=%d dirs=%d\n",
			u.UDFRevision, u.VolumeID, u.PartitionStart, u.PartitionBlocks, u.FileCount, u.DirCount)
		// Show the largest files (the ones most likely to hit the short_ad limit).
		files := append([]UDFFile(nil), u.Files...)
		sort.Slice(files, func(i, j int) bool { return files[i].Size > files[j].Size })
		for i := 0; i < len(files) && i < 5; i++ {
			f := files[i]
			fmt.Fprintf(&b, "  - %-40s %12d bytes  %d extent(s)\n", f.Path, f.Size, f.Extents)
		}
	}

	switch {
	case len(r.Issues) == 0:
		b.WriteString("OK: no issues found\n")
	default:
		fmt.Fprintf(&b, "%d issue(s):\n", len(r.Issues))
		for _, is := range r.Issues {
			fmt.Fprintf(&b, "  %s\n", is)
		}
	}
	return b.String()
}
