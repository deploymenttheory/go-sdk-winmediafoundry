// Example 07_wim_tree: opens a downloaded WIM/ESD, lists its images, and prints
// the directory tree of one image (decompressing that image's metadata, which
// for an ESD is LZMS-compressed — handled entirely in pure Go).
//
// Run:
//
//	go run ./examples/07_wim_tree /path/to/install.esd [imageIndex]
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/pkg/wim"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: %s <install.esd> [imageIndex]", os.Args[0])
	}
	index := 1
	if len(os.Args) > 2 {
		index, _ = strconv.Atoi(os.Args[2])
	}

	w, err := wim.Open(os.Args[1])
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer w.Close()

	for _, im := range w.Images() {
		marker := " "
		if im.Index == index {
			marker = "*"
		}
		fmt.Printf(" %s [%d] %s (%s, %s)\n", marker, im.Index, im.Name, im.Edition, im.Architecture)
	}
	fmt.Printf("\nDirectory tree of image %d:\n", index)

	root, err := w.OpenImage(index)
	if err != nil {
		log.Fatalf("OpenImage(%d): %v", index, err)
	}

	var files, dirs int
	root.Walk(func(path string, f *File) {
		if f.IsDir() {
			dirs++
		} else {
			files++
		}
		// Print only the first three levels to keep output manageable.
		if depth(path) <= 3 {
			kind := "f"
			if f.IsDir() {
				kind = "d"
			}
			fmt.Printf("  %s %d\t%s\n", kind, f.Size, path)
		}
	})
	fmt.Printf("\nTotal: %d directories, %d files\n", dirs, files)
}

// File is re-exported locally for brevity in the Walk callback.
type File = wim.File

func depth(path string) int {
	n := 1
	for _, c := range path {
		if c == '/' {
			n++
		}
	}
	return n
}
