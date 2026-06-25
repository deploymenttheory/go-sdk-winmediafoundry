// Throwaway: master a media directory into a Windows UDF ISO via the SDK writer.
// usage: remaster <mediaDir> <out.iso> <volumeID>
package main

import (
	"fmt"
	"os"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/pkg/iso"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Println("usage: remaster <mediaDir> <out.iso> <volumeID>")
		os.Exit(2)
	}
	if err := iso.BuildWindowsUDF(os.Args[1], os.Args[2], os.Args[3]); err != nil {
		fmt.Fprintln(os.Stderr, "ERR:", err)
		os.Exit(1)
	}
	fmt.Println("BUILT", os.Args[2])
}
