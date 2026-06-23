package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	filesapi "github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/api/files"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/constants"
)

var filesCmd = &cobra.Command{
	Use:   "files",
	Short: "List files for a Windows build",
	Long:  "Resolve the file list (and optionally pre-signed CDN URLs) for a build.",
	RunE:  runFiles,
}

func init() {
	filesCmd.Flags().String("build", "", "build version to resolve, e.g. 26100.4061 (defaults to the latest match)")
	filesCmd.Flags().String("language", "", "filter by BCP-47 language tag, e.g. en-us")
	filesCmd.Flags().String("edition", "", "filter by Windows edition, e.g. Professional")
	filesCmd.Flags().String("extension", "", "filter by file extension, e.g. .esd or .cab")
	filesCmd.Flags().Bool("cdn-urls", false, "resolve live pre-signed CDN download URLs")
	rootCmd.AddCommand(filesCmd)
}

func fileOptionsFromFlags(cmd *cobra.Command) []filesapi.FileOption {
	var opts []filesapi.FileOption
	if cdn, _ := cmd.Flags().GetBool("cdn-urls"); cdn {
		opts = append(opts, filesapi.WithCDNURLs())
	}
	if lang, _ := cmd.Flags().GetString("language"); lang != "" {
		opts = append(opts, filesapi.WithLanguage(lang))
	}
	if ed, _ := cmd.Flags().GetString("edition"); ed != "" {
		opts = append(opts, filesapi.WithEdition(constants.Edition(strings.ToUpper(ed))))
	}
	if ext, _ := cmd.Flags().GetString("extension"); ext != "" {
		opts = append(opts, filesapi.WithExtension(ext))
	}
	return opts
}

func runFiles(cmd *cobra.Command, _ []string) error {
	c, err := newWUClient()
	if err != nil {
		return err
	}
	buildFilter, _ := cmd.Flags().GetString("build")
	build, err := resolveBuild(cmd.Context(), c, buildFilter)
	if err != nil {
		return err
	}

	files, _, err := c.Files.GetFiles(cmd.Context(), build, fileOptionsFromFlags(cmd)...)
	if err != nil {
		return fmt.Errorf("get files: %w", err)
	}

	fmt.Printf("Build %s (%s): %d files\n", build.Build, build.Arch, len(files))
	w := newTable()
	fmt.Fprintln(w, "NAME\tSIZE (MB)\tTYPE\tURL")
	for _, f := range files {
		url := f.URL
		if url == "" {
			url = "-"
		}
		fmt.Fprintf(w, "%s\t%.1f\t%s\t%s\n", f.Name, float64(f.SizeBytes)/1e6, f.FileType, url)
	}
	return w.Flush()
}
