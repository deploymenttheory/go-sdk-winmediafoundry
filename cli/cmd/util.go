package cmd

import (
	"os"
	"text/tabwriter"
)

// newTable returns a tab-aligned writer to stdout.
func newTable() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
}
