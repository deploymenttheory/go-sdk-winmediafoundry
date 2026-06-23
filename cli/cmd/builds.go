package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var buildsCmd = &cobra.Command{
	Use:   "builds",
	Short: "List available Windows builds from Windows Update",
	Long:  "Discover Windows builds for the configured architecture, ring, and SKU.",
	RunE:  runBuilds,
}

func init() {
	buildsCmd.Flags().String("build", "", "filter to a specific build version, e.g. 26100.4061")
	rootCmd.AddCommand(buildsCmd)
}

func runBuilds(cmd *cobra.Command, _ []string) error {
	c, err := newWUClient()
	if err != nil {
		return err
	}
	buildFilter, _ := cmd.Flags().GetString("build")
	opts, err := wuFetchOptions(buildFilter)
	if err != nil {
		return err
	}

	builds, _, err := c.Builds.FetchBuilds(cmd.Context(), opts...)
	if err != nil {
		return fmt.Errorf("fetch builds: %w", err)
	}
	if len(builds) == 0 {
		fmt.Println("no builds found")
		return nil
	}

	w := newTable()
	fmt.Fprintln(w, "BUILD\tARCH\tRING\tSTABLE\tTITLE")
	for _, b := range builds {
		fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%s\n", b.Build, b.Arch, b.Ring, b.IsStable, b.Title)
	}
	return w.Flush()
}
