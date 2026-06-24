package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Build information, overridden at release time via -ldflags by GoReleaser:
//
//	-X github.com/deploymenttheory/go-sdk-winmediafoundry/cli/cmd.version=...
//	-X github.com/deploymenttheory/go-sdk-winmediafoundry/cli/cmd.commit=...
//	-X github.com/deploymenttheory/go-sdk-winmediafoundry/cli/cmd.date=...
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the winmediafoundry version",
	RunE: func(cmd *cobra.Command, _ []string) error {
		_, err := fmt.Printf("winmediafoundry %s (commit %s, built %s, %s/%s, %s)\n",
			version, commit, date, runtime.GOOS, runtime.GOARCH, runtime.Version())
		return err
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.Version = version
}
