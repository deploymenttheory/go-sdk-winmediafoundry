package wim

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var errNotRegularFile = errors.New("wim: not a regular file")

// blobBytes returns the uncompressed bytes for a blob location.
func (w *WIM) blobBytes(loc blobLocation) ([]byte, error) {
	if loc.solid != nil {
		return loc.solid.readAt(loc.offset, loc.size)
	}
	if loc.rd.compressed() {
		rc, err := w.resourceReader(loc.rd)
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return w.readResourceRaw(loc.rd)
}

// ReadFile returns the full contents of a regular file within an image. It
// returns an empty slice for an empty file (zero hash).
func (w *WIM) ReadFile(f *File) ([]byte, error) {
	if f.IsDir() {
		return nil, errNotRegularFile
	}
	if isZeroHash(f.Hash) {
		return []byte{}, nil // empty file
	}
	if err := w.loadOffsetTable(); err != nil {
		return nil, err
	}
	loc, ok := w.blobs[f.Hash]
	if !ok {
		return nil, fmt.Errorf("wim: no blob found for file %q", f.Name)
	}
	return w.blobBytes(loc)
}

// ExtractImage extracts the 1-based image index to destDir, creating files and
// directories. Security descriptors, alternate data streams, and reparse points
// are not applied.
func (w *WIM) ExtractImage(index int, destDir string) error {
	root, err := w.OpenImage(index)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("wim: create %s: %w", destDir, err)
	}
	return w.extractChildren(root, destDir)
}

func (w *WIM) extractChildren(dir *File, dest string) error {
	for _, c := range dir.Children() {
		target := filepath.Join(dest, c.Name) //nolint:gosec // names come from a trusted image
		switch {
		case c.IsDir():
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("wim: mkdir %s: %w", target, err)
			}
			if err := w.extractChildren(c, target); err != nil {
				return err
			}
		case c.Attributes&attrReparsePoint != 0:
			// Skip reparse points (symlinks/junctions) for now.
		default:
			data, err := w.ReadFile(c)
			if err != nil {
				return err
			}
			if err := os.WriteFile(target, data, 0o644); err != nil { //nolint:gosec // image file perms
				return fmt.Errorf("wim: write %s: %w", target, err)
			}
		}
	}
	return nil
}
