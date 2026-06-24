// Example 08_wim_extract: extracts an image from a WIM/ESD to a directory. File
// contents are read from the image's solid LZMS resources and decompressed in
// pure Go. Extracting a full Windows image writes several GB and takes a while.
//
// Run:
//
//	go run ./examples/08_wim_extract /path/to/install.esd <imageIndex> <destDir>
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/pkg/wim"
)

func main() {
	if len(os.Args) < 4 {
		log.Fatalf("usage: %s <install.esd> <imageIndex> <destDir>", os.Args[0])
	}
	index, err := strconv.Atoi(os.Args[2])
	if err != nil {
		log.Fatalf("invalid image index: %v", err)
	}
	dest := os.Args[3]

	w, err := wim.Open(os.Args[1])
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer w.Close()

	for _, im := range w.Images() {
		if im.Index == index {
			fmt.Printf("Extracting image %d: %s (%s, %s) -> %s\n",
				im.Index, im.Name, im.Edition, im.Architecture, dest)
		}
	}

	start := time.Now()
	if err := w.ExtractImage(index, dest); err != nil {
		log.Fatalf("extract: %v", err)
	}
	fmt.Printf("Done in %s\n", time.Since(start).Round(time.Second))
}
