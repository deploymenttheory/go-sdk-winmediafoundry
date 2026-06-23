package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Compare the file sets of two Windows builds",
	RunE:  runDiff,
}

func init() {
	diffCmd.Flags().String("base", "", "base build version")
	diffCmd.Flags().String("target", "", "target build version")
	_ = diffCmd.MarkFlagRequired("base")
	_ = diffCmd.MarkFlagRequired("target")
	rootCmd.AddCommand(diffCmd)
}

func runDiff(cmd *cobra.Command, _ []string) error {
	c, err := newWUClient()
	if err != nil {
		return err
	}
	baseFilter, _ := cmd.Flags().GetString("base")
	targetFilter, _ := cmd.Flags().GetString("target")

	base, err := resolveBuild(cmd.Context(), c, baseFilter)
	if err != nil {
		return fmt.Errorf("base: %w", err)
	}
	target, err := resolveBuild(cmd.Context(), c, targetFilter)
	if err != nil {
		return fmt.Errorf("target: %w", err)
	}

	d, _, err := c.Diff.Diff(cmd.Context(), base, target)
	if err != nil {
		return fmt.Errorf("diff: %w", err)
	}

	fmt.Printf("%s -> %s\n", base.Build, target.Build)
	fmt.Printf("  added:     %d\n", len(d.Added))
	fmt.Printf("  removed:   %d\n", len(d.Removed))
	fmt.Printf("  changed:   %d\n", len(d.Changed))
	fmt.Printf("  unchanged: %d\n", d.Unchanged)
	return nil
}
