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
	"os"
	"path/filepath"
	"strings"

	"github.com/deploymenttheory/winmediafoundry/pkg/iso"
	"github.com/deploymenttheory/winmediafoundry/pkg/wim"
)

// Options configures an ISO build.
type Options struct {
	VolumeID string
	// WorkDir is the scratch directory for the media tree and temporary
	// extractions. Empty uses a fresh os.MkdirTemp directory (removed on success).
	WorkDir string
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

	media := filepath.Join(work, "media")
	if err := w.ExtractImage(classes.setupMedia, media); err != nil {
		return fmt.Errorf("builder: extract setup media: %w", err)
	}

	sources := filepath.Join(media, "sources")
	if err := os.MkdirAll(sources, 0o755); err != nil {
		return fmt.Errorf("builder: %w", err)
	}

	if err := buildWIM(w, classes.bootImages, filepath.Join(sources, "boot.wim")); err != nil {
		return fmt.Errorf("builder: boot.wim: %w", err)
	}
	if len(classes.editions) > 0 {
		if err := buildWIM(w, classes.editions, filepath.Join(sources, "install.wim")); err != nil {
			return fmt.Errorf("builder: install.wim: %w", err)
		}
	}

	if err := iso.BuildWindowsUDF(media, outISOPath, opts.VolumeID); err != nil {
		return err
	}
	return nil
}

// buildWIM writes the given source images as the images of a new uncompressed
// WIM at outPath, preserving order. Images are copied directly from the source
// WIM (no extraction to disk), which preserves file attributes/timestamps.
func buildWIM(w *wim.WIM, indices []int, outPath string) error {
	out, err := os.Create(outPath) //nolint:gosec // caller-controlled path
	if err != nil {
		return err
	}
	defer out.Close()

	ww, err := wim.NewWriter(out)
	if err != nil {
		return err
	}
	for _, idx := range indices {
		if err := ww.AddImageFromWIM(w, idx, imageName(w, idx)); err != nil {
			return fmt.Errorf("copy image %d: %w", idx, err)
		}
	}
	return ww.Close()
}

func imageName(w *wim.WIM, index int) string {
	for _, im := range w.Images() {
		if im.Index == index {
			return im.Name
		}
	}
	return fmt.Sprintf("Image %d", index)
}
