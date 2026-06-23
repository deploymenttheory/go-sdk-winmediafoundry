package wim

import (
	"crypto/sha1" //nolint:gosec // WIM blobs are content-addressed by SHA-1
)

// AddImageFromWIM copies the 1-based image index from src into the WIM being
// written, as a new image named name. Blob data is streamed directly from src
// (no extraction to disk), deduplicated by SHA-1, and the directory tree is
// rebuilt preserving file attributes and the three timestamps.
func (w *Writer) AddImageFromWIM(src *WIM, index int, name string) error {
	root, err := src.OpenImage(index)
	if err != nil {
		return err
	}
	rec := imageRec{name: name}
	node, err := w.copyNode(src, root, &rec)
	if err != nil {
		return err
	}

	meta := buildMetadata(node)
	rec.metaOffset = w.pos
	if err := w.write(meta); err != nil {
		return err
	}
	rec.metaSize = int64(len(meta))
	rec.metaHash = sha1.Sum(meta) //nolint:gosec
	w.images = append(w.images, rec)
	return nil
}

// copyNode converts a source File into a writeNode, writing its blob (for
// regular files) into the destination WIM if not already present.
func (w *Writer) copyNode(src *WIM, f *File, rec *imageRec) (*writeNode, error) {
	node := &writeNode{
		name:       f.Name,
		isDir:      f.IsDir(),
		attrs:      f.Attributes,
		createTime: f.CreationTime,
		accessTime: f.LastAccessTime,
		writeTime:  f.LastWriteTime,
		hash:       f.Hash,
	}

	if f.IsDir() {
		rec.dirCount++
		for _, c := range f.Children() {
			child, err := w.copyNode(src, c, rec)
			if err != nil {
				return nil, err
			}
			node.children = append(node.children, child)
		}
		return node, nil
	}

	rec.fileCount++
	if isZeroHash(f.Hash) || w.seen[f.Hash] {
		return node, nil // empty file, or a blob already written for this content
	}
	data, err := src.ReadFile(f)
	if err != nil {
		return nil, err
	}
	w.seen[f.Hash] = true
	offset := w.pos
	if err := w.write(data); err != nil {
		return nil, err
	}
	w.blobs = append(w.blobs, blobRec{hash: f.Hash, offset: offset, size: int64(len(data))})
	rec.totalBytes += int64(len(data))
	return node, nil
}
