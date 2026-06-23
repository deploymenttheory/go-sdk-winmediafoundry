package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	filesapi "github.com/deploymenttheory/winmediafoundry/windowsuup/api/files"
	"github.com/deploymenttheory/winmediafoundry/windowsuup/constants"
)

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download a build's files from the CDN",
	Long:  "Resolve pre-signed CDN URLs for a build's files and stream them to a directory.",
	RunE:  runDownload,
}

func init() {
	downloadCmd.Flags().String("build", "", "build version to download, e.g. 26100.4061")
	downloadCmd.Flags().String("language", "", "filter by BCP-47 language tag, e.g. en-us")
	downloadCmd.Flags().String("edition", "", "filter by Windows edition, e.g. Professional")
	downloadCmd.Flags().String("extension", "", "filter by file extension, e.g. .esd")
	downloadCmd.Flags().StringP("out", "o", ".", "destination directory")
	downloadCmd.Flags().Int("concurrency", 4, "parallel downloads")
	rootCmd.AddCommand(downloadCmd)
}

func runDownload(cmd *cobra.Command, _ []string) error {
	c, err := newWUClient()
	if err != nil {
		return err
	}
	buildFilter, _ := cmd.Flags().GetString("build")
	build, err := resolveBuild(cmd.Context(), c, buildFilter)
	if err != nil {
		return err
	}

	// CDN URLs are required to download.
	opts := []filesapi.FileOption{filesapi.WithCDNURLs()}
	if lang, _ := cmd.Flags().GetString("language"); lang != "" {
		opts = append(opts, filesapi.WithLanguage(lang))
	}
	if ed, _ := cmd.Flags().GetString("edition"); ed != "" {
		opts = append(opts, filesapi.WithEdition(constants.Edition(strings.ToUpper(ed))))
	}
	if ext, _ := cmd.Flags().GetString("extension"); ext != "" {
		opts = append(opts, filesapi.WithExtension(ext))
	}

	files, _, err := c.Files.GetFiles(cmd.Context(), build, opts...)
	if err != nil {
		return fmt.Errorf("get files: %w", err)
	}
	if len(files) == 0 {
		fmt.Println("no files matched the filters")
		return nil
	}

	out, _ := cmd.Flags().GetString("out")
	concurrency, _ := cmd.Flags().GetInt("concurrency")
	fmt.Printf("Downloading %d files to %s ...\n", len(files), out)
	if err := c.Download.DownloadFiles(cmd.Context(), files, out, concurrency); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	fmt.Println("done")
	return nil
}
