// Example 06_wim_info: opens a downloaded WIM/ESD file and prints its header
// summary and image catalog. No decompression is required to enumerate images.
//
// Run:
//
//	go run ./examples/06_wim_info /path/to/install.esd
package main

import (
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/pkg/wim"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: %s <install.esd|install.wim>", os.Args[0])
	}

	w, err := wim.Open(os.Args[1])
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer w.Close()

	info := w.Info()
	fmt.Printf("Compression: %s\nChunk size:  %d\nImages:      %d\nSolid:       %v\n\n",
		info.Compression, info.ChunkSize, info.ImageCount, info.Solid)

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "IDX\tNAME\tEDITION\tARCH\tFILES\tSIZE (GB)")
	for _, im := range w.Images() {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%d\t%.2f\n",
			im.Index, im.Name, im.Edition, im.Architecture, im.FileCount,
			float64(im.TotalBytes)/1e9)
	}
	tw.Flush()
}
