// Throwaway: build an autounattend-embedded ARM64 install ISO from a cached ESD.
// usage: embedone <esd> <out.iso> <autounattend.xml> <workdir>
package main

import (
	"fmt"
	"os"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/pkg/builder"
)

func main() {
	esd, out, ua, work := os.Args[1], os.Args[2], os.Args[3], os.Args[4]
	xml, err := os.ReadFile(ua)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read autounattend:", err)
		os.Exit(1)
	}
	if err := builder.BuildISO(esd, out, builder.Options{
		VolumeID:   "CCCOMA_ARM64FRE",
		WorkDir:    work,
		Progress:   os.Stdout,
		ExtraFiles: map[string][]byte{"autounattend.xml": xml},
	}); err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		os.Exit(1)
	}
	fmt.Println("BUILT", out)
}
