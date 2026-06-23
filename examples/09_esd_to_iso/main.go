// Example 09_esd_to_iso: builds a bootable Windows installation ISO from a
// downloaded ESD, entirely in pure Go (no wimlib/oscdimg). It extracts the
// Setup Media skeleton, rebuilds sources/boot.wim and sources/install.wim from
// the ESD's images, and masters a UDF + El Torito ISO.
//
// Run:
//
//	go run ./examples/09_esd_to_iso /path/to/install.esd out.iso [VOLUME_LABEL]
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/pkg/builder"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatalf("usage: %s <install.esd> <out.iso> [volume label]", os.Args[0])
	}
	esd, out := os.Args[1], os.Args[2]
	label := "CCCOMA_X64FRE"
	if len(os.Args) > 3 {
		label = os.Args[3]
	}

	fmt.Printf("Building %s from %s ...\n", out, esd)
	start := time.Now()
	if err := builder.BuildISO(esd, out, builder.Options{VolumeID: label}); err != nil {
		log.Fatalf("build: %v", err)
	}
	if fi, err := os.Stat(out); err == nil {
		fmt.Printf("Done in %s (%.2f GB)\n", time.Since(start).Round(time.Second), float64(fi.Size())/1e9)
	}
}
