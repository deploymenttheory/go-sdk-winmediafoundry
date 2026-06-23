// Command winmediafoundry is a CLI for acquiring and building Windows
// installation media: discover Windows Update builds, browse the ESD catalog,
// read and extract WIM/ESD images, and master bootable ISOs.
package main

import "github.com/deploymenttheory/go-sdk-winmediafoundry/cli/cmd"

func main() {
	cmd.Execute()
}
