package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload"
	sdapi "github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/api/softwaredownload"
	sdconst "github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/constants"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload/shared/models"
)

var swdlCmd = &cobra.Command{
	Use:   "swdl",
	Short: "Download consumer Windows 11 ISOs from Microsoft's software-download site",
	Long: `swdl drives Microsoft's consumer software-download flow: it scrapes the public
Windows 11 ISO download pages, resolves a signed, time-limited download link for a
chosen edition and language, and streams the multi-edition ISO to disk.

This is distinct from the Windows Update 'download' command and the 'esd'/'iso
build' path — it fetches the same consumer ISOs the browser download page serves.`,
}

var swdlListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available Windows 11 ISO editions",
	Long:  "Scrape the Windows 11 software-download pages and list the product editions they advertise.",
	RunE:  runSWDLList,
}

var swdlResolveCmd = &cobra.Command{
	Use:   "resolve <name-or-id>",
	Short: "Resolve a signed ISO download link without downloading",
	Long: `Resolve the signed, time-limited download link for an edition without
downloading it. The argument is matched as a product-edition id when it is all
digits (e.g. 3324), otherwise as a case-insensitive substring of the edition name
(e.g. "Arm64").`,
	Args: cobra.ExactArgs(1),
	RunE: runSWDLResolve,
}

var swdlDownloadCmd = &cobra.Command{
	Use:   "download <name-or-id>",
	Short: "Resolve and download a Windows 11 ISO",
	Long: `Resolve a signed download link and stream the ISO to disk. The argument is
matched as a product-edition id when it is all digits (e.g. 3324), otherwise as a
case-insensitive substring of the edition name (e.g. "Arm64").

The ISO is written atomically; re-running skips a file that already exists at the
expected size.`,
	Args: cobra.ExactArgs(1),
	RunE: runSWDLDownload,
}

func init() {
	swdlListCmd.Flags().String("architecture", "", "filter by architecture: x64 or arm64 (default: all)")
	swdlListCmd.Flags().String("locale", "en-US", "page/connector locale")

	swdlResolveCmd.Flags().String("architecture", "", "architecture to resolve: x64 or arm64")
	swdlResolveCmd.Flags().String("language", "", `ISO language, e.g. "en-US" or "English (United States)" (default: English (United States))`)
	swdlResolveCmd.Flags().String("locale", "en-US", "page/connector locale")

	swdlDownloadCmd.Flags().StringP("out", "o", ".", "destination directory")
	swdlDownloadCmd.Flags().String("architecture", "", "architecture to download: x64 or arm64")
	swdlDownloadCmd.Flags().String("language", "", `ISO language, e.g. "en-US" or "English (United States)" (default: English (United States))`)
	swdlDownloadCmd.Flags().String("locale", "en-US", "page/connector locale")
	swdlDownloadCmd.Flags().Bool("progress", true, "show a download progress bar")

	swdlCmd.AddCommand(swdlListCmd, swdlResolveCmd, swdlDownloadCmd)
	rootCmd.AddCommand(swdlCmd)
}

func runSWDLList(cmd *cobra.Command, _ []string) error {
	c, err := newSWDLClient()
	if err != nil {
		return err
	}

	opts := []sdapi.Option{sdapi.WithLocale(mustString(cmd, "locale"))}
	archOpt, err := swdlArchOption(cmd)
	if err != nil {
		return err
	}
	if archOpt != nil {
		opts = append(opts, archOpt)
	}

	products, _, err := c.List(cmd.Context(), opts...)
	if err != nil {
		return fmt.Errorf("list editions: %w", err)
	}
	if len(products) == 0 {
		fmt.Println("no product editions found")
		return nil
	}

	w := newTable()
	fmt.Fprintln(w, "ID\tNAME\tARCH\tPAGE URL")
	for _, p := range products {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.EditionID, p.Name, p.Arch, p.PageURL)
	}
	return w.Flush()
}

func runSWDLResolve(cmd *cobra.Command, args []string) error {
	c, err := newSWDLClient()
	if err != nil {
		return err
	}

	opts, err := swdlResolveOptions(cmd)
	if err != nil {
		return err
	}

	link, err := resolveSWDLLink(cmd.Context(), c, args[0], opts)
	if err != nil {
		return err
	}

	printSWDLLink(link)
	return nil
}

func runSWDLDownload(cmd *cobra.Command, args []string) error {
	c, err := newSWDLClient()
	if err != nil {
		return err
	}

	opts, err := swdlResolveOptions(cmd)
	if err != nil {
		return err
	}
	opts = append(opts, sdapi.WithDownloadDir(mustString(cmd, "out")))
	if progress, _ := cmd.Flags().GetBool("progress"); progress {
		opts = append(opts, sdapi.WithProgress(nil))
	}

	link, err := resolveSWDLLink(cmd.Context(), c, args[0], opts)
	if err != nil {
		return err
	}

	fmt.Printf("\nDownloaded %s to %s\n", link.FileName, link.LocalPath)
	return nil
}

// swdlArchOption returns the WithArch option for the --architecture flag, or nil
// when the flag is empty. It errors on an unrecognised architecture token.
func swdlArchOption(cmd *cobra.Command) (sdapi.Option, error) {
	raw := mustString(cmd, "architecture")
	if raw == "" {
		return nil, nil
	}
	arch := sdconst.ArchFromString(raw)
	if arch == "" {
		return nil, fmt.Errorf("invalid --architecture %q (want x64 or arm64)", raw)
	}
	return sdapi.WithArch(arch), nil
}

// swdlResolveOptions builds the scrape/resolution options shared by the resolve
// and download commands from the locale, language, and architecture flags.
func swdlResolveOptions(cmd *cobra.Command) ([]sdapi.Option, error) {
	opts := []sdapi.Option{sdapi.WithLocale(mustString(cmd, "locale"))}
	if lang := mustString(cmd, "language"); lang != "" {
		opts = append(opts, sdapi.WithLanguage(lang))
	}
	archOpt, err := swdlArchOption(cmd)
	if err != nil {
		return nil, err
	}
	if archOpt != nil {
		opts = append(opts, archOpt)
	}
	return opts, nil
}

// resolveSWDLLink dispatches to GetByID for an all-digit edition id, or GetByName
// for a name substring.
func resolveSWDLLink(ctx context.Context, c *softwaredownload.Client, arg string, opts []sdapi.Option) (*models.DownloadLink, error) {
	var (
		link *models.DownloadLink
		err  error
	)
	if digitsOnly(arg) {
		link, _, err = c.GetByID(ctx, arg, opts...)
	} else {
		link, _, err = c.GetByName(ctx, arg, opts...)
	}
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", arg, err)
	}
	return link, nil
}

// printSWDLLink prints the resolved download link details.
func printSWDLLink(link *models.DownloadLink) {
	w := newTable()
	fmt.Fprintf(w, "File:\t%s\n", link.FileName)
	fmt.Fprintf(w, "Arch:\t%s\n", link.Arch)
	fmt.Fprintf(w, "Language:\t%s\n", link.Language.LocalizedName)
	if link.SizeBytes > 0 {
		fmt.Fprintf(w, "Size:\t%.2f GB\n", float64(link.SizeBytes)/1e9)
	} else {
		fmt.Fprintf(w, "Size:\t%s\n", "unknown")
	}
	if !link.ExpiresAt.IsZero() {
		fmt.Fprintf(w, "Expires:\t%s\n", link.ExpiresAt.UTC().Format(time.RFC3339))
	}
	fmt.Fprintf(w, "URL:\t%s\n", link.URL)
	_ = w.Flush()
}

// digitsOnly reports whether s is non-empty and made up entirely of ASCII digits.
func digitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
