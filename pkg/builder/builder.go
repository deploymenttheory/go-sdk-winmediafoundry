// Package builder orchestrates the full ESD→ISO pipeline: it reads a Windows
// ESD/WIM, extracts the Setup Media skeleton, rebuilds sources/boot.wim and
// sources/install.wim from the ESD's images, and masters a bootable UDF +
// El Torito ISO.
//
// Because the output media uses UDF (no ISO9660 4 GiB-per-file limit), the WIMs
// are written uncompressed; the resulting ISO is therefore larger than a
// Microsoft-produced one, but valid and bootable.
package builder

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/pkg/iso"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/pkg/progress_counter"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/pkg/wim"
)

// Options configures an ISO build.
type Options struct {
	VolumeID string
	// WorkDir is the scratch directory for the media tree and temporary
	// extractions. Empty uses a fresh os.MkdirTemp directory (removed on success).
	WorkDir string
	// Progress, when non-nil, receives a terminal progress bar for the slow
	// phases (rebuilding boot.wim / install.wim). nil builds silently.
	Progress io.Writer
	// ExtraFiles maps ISO-relative, slash-separated paths to file content, staged
	// into the media tree just before mastering. A file mapped to "autounattend.xml"
	// lands at the ISO root, where Windows Setup auto-detects it for an unattended
	// install. Keys are anchored and cleaned so they cannot escape the media root;
	// intermediate directories are created as needed.
	ExtraFiles map[string][]byte
}

// BuildISO assembles a bootable Windows ISO at outISOPath from the ESD/WIM at
// esdPath.
func BuildISO(esdPath, outISOPath string, opts Options) error {
	w, err := wim.Open(esdPath)
	if err != nil {
		return err
	}
	defer w.Close()
	return BuildISOFromWIM(w, outISOPath, opts)
}

// imageClasses groups an ESD's images by role.
type imageClasses struct {
	setupMedia int
	bootImages []int // Windows PE, then Windows Setup
	editions   []int
}

// classify assigns each image a role from its catalog name.
func classify(images []wim.ImageInfo) imageClasses {
	var c imageClasses
	for _, im := range images {
		name := strings.ToLower(im.Name)
		switch {
		case strings.Contains(name, "setup media"):
			c.setupMedia = im.Index
		case strings.Contains(name, "windows pe"):
			c.bootImages = append(c.bootImages, im.Index)
		case strings.Contains(name, "windows setup"):
			c.bootImages = append(c.bootImages, im.Index)
		default:
			c.editions = append(c.editions, im.Index)
		}
	}
	return c
}

// BuildISOFromWIM assembles the ISO from an already-opened ESD/WIM.
func BuildISOFromWIM(w *wim.WIM, outISOPath string, opts Options) error {
	classes := classify(w.Images())
	if classes.setupMedia == 0 {
		return fmt.Errorf("builder: no \"Windows Setup Media\" image found")
	}
	if len(classes.bootImages) == 0 {
		return fmt.Errorf("builder: no Windows PE/Setup images found for boot.wim")
	}

	work := opts.WorkDir
	if work == "" {
		var err error
		work, err = os.MkdirTemp("", "windowsuup-iso-")
		if err != nil {
			return fmt.Errorf("builder: workdir: %w", err)
		}
		defer os.RemoveAll(work)
	}

	progressf(opts.Progress, "Extracting setup media...\n")
	media := filepath.Join(work, "media")
	if err := w.ExtractImage(classes.setupMedia, media); err != nil {
		return fmt.Errorf("builder: extract setup media: %w", err)
	}

	sources := filepath.Join(media, "sources")
	if err := os.MkdirAll(sources, 0o755); err != nil {
		return fmt.Errorf("builder: %w", err)
	}

	// boot.wim and install.wim are compressed with LZX to match Microsoft media
	// and satisfy go-winio's WIM reader, which rejects XPRESS-flagged WIMs as
	// unsupported (supportedHdrFlags only includes hdrFlagCompressLzx).
	//
	// The classify step orders bootImages as [Windows PE, Windows Setup].
	// Windows Setup (the last image) is the bootable one, so bootIndex equals
	// len(bootImages). bootmgr reads BootIndex from the WIM header to find the
	// boot image and BootMetadata to locate its metadata resource.
	if err := buildWIM(w, classes.bootImages, filepath.Join(sources, "boot.wim"), wim.CompressionLZX, len(classes.bootImages), opts.Progress); err != nil {
		return fmt.Errorf("builder: boot.wim: %w", err)
	}
	if len(classes.editions) > 0 {
		if err := buildWIM(w, classes.editions, filepath.Join(sources, "install.wim"), wim.CompressionLZX, 0, opts.Progress); err != nil {
			return fmt.Errorf("builder: install.wim: %w", err)
		}
	}

	if err := injectExtraFiles(media, opts.ExtraFiles); err != nil {
		return fmt.Errorf("builder: inject extra files: %w", err)
	}

	progressf(opts.Progress, "Mastering ISO...\n")
	if err := iso.BuildWindowsUDF(media, outISOPath, opts.VolumeID); err != nil {
		return err
	}
	return nil
}

// injectExtraFiles writes opts.ExtraFiles into the staged media tree (rooted at
// mediaRoot) before the ISO is mastered, so the UDF writer that walks the tree
// picks them up. Keys are ISO-relative, slash-separated paths; each is anchored
// at "/" and cleaned, so ".." cannot escape mediaRoot. Intermediate directories
// are created. A nil/empty map is a no-op.
func injectExtraFiles(mediaRoot string, extras map[string][]byte) error {
	rootPrefix := filepath.Clean(mediaRoot) + string(os.PathSeparator)
	for rel, content := range extras {
		clean := filepath.Clean("/" + filepath.FromSlash(rel))
		dst := filepath.Join(mediaRoot, clean)
		if !strings.HasPrefix(dst, rootPrefix) {
			return fmt.Errorf("extra file %q escapes media root", rel)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, content, 0o644); err != nil { //nolint:gosec // staged media files
			return err
		}
	}
	return nil
}

// buildWIM writes the given source images as the images of a new WIM at
// outPath, preserving order. Images are copied directly from the source WIM (no
// extraction to disk), which preserves file attributes/timestamps. bootIndex is
// the 1-based output image number to mark as bootable (0 for none). When
// progress is non-nil, the (slow) write is reported via a progress bar sized to
// the images' uncompressed bytes.
func buildWIM(w *wim.WIM, indices []int, outPath string, comp wim.Compression, bootIndex int, progress io.Writer) error {
	out, err := os.Create(outPath) //nolint:gosec // caller-controlled path
	if err != nil {
		return err
	}
	defer out.Close()

	var dst io.WriteSeeker = out
	if progress != nil {
		bar := progress_counter.NewWithLabel(progress, "Building")
		dst = bar.WriteSeeker(filepath.Base(outPath), out, sumImageBytes(w, indices))
	}

	ww, err := wim.NewWriterCompressed(dst, comp)
	if err != nil {
		return err
	}
	ww.SetBootIndex(bootIndex)
	for _, idx := range indices {
		if err := ww.AddImageFromWIM(w, idx, imageName(w, idx)); err != nil {
			return fmt.Errorf("copy image %d: %w", idx, err)
		}
	}
	return ww.Close()
}

// sumImageBytes totals the uncompressed size of the given images, used as the
// progress-bar denominator. Returns 0 when unknown (the bar then shows bytes +
// speed only).
func sumImageBytes(w *wim.WIM, indices []int) int64 {
	want := make(map[int]bool, len(indices))
	for _, idx := range indices {
		want[idx] = true
	}
	var total int64
	for _, im := range w.Images() {
		if want[im.Index] {
			total += im.TotalBytes
		}
	}
	return total
}

// progressf writes a status line to w when it is non-nil.
func progressf(w io.Writer, format string, args ...any) {
	if w != nil {
		fmt.Fprintf(w, format, args...)
	}
}

func imageName(w *wim.WIM, index int) string {
	for _, im := range w.Images() {
		if im.Index == index {
			return im.Name
		}
	}
	return fmt.Sprintf("Image %d", index)
}
