package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/pkg/isoinspect"
)

func main() {
	for _, p := range os.Args[1:] {
		rep, err := isoinspect.Inspect(p)
		if err != nil {
			fmt.Println("ERR", p, err)
			continue
		}
		fmt.Printf("\n===== %s (%.2f GB) OK=%v =====\n", p, float64(rep.Size)/1e9, rep.OK())
		if rep.ElTorito != nil {
			fmt.Printf("ElTorito: %+v\n", *rep.ElTorito)
		}
		if u := rep.UDF; u != nil {
			fmt.Printf("UDF: present=%v rev=%#04x vol=%q partStart=%d partBlocks=%d rootFE=%d files=%d dirs=%d\n",
				u.Present, u.UDFRevision, u.VolumeID, u.PartitionStart, u.PartitionBlocks, u.RootFEBlock, u.FileCount, u.DirCount)
			for _, f := range u.Files {
				l := strings.ToLower(f.Path)
				if strings.Contains(l, "bootaa64") || strings.Contains(l, ".wim") || f.Size > 1<<30 {
					fmt.Printf("  FILE %-48s size=%-12d extents=%d\n", f.Path, f.Size, f.Extents)
				}
			}
		}
		errs := rep.Errors()
		fmt.Printf("ERRORS (%d):\n", len(errs))
		for _, i := range errs {
			fmt.Println("   ", i)
		}
		warns := rep.Warnings()
		fmt.Printf("WARNINGS (%d):\n", len(warns))
		for n, w := range warns {
			if n < 12 {
				fmt.Println("   ", w)
			}
		}
	}
}
