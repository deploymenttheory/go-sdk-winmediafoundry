package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/pkg/isoinspect"
)

var isoInspectCmd = &cobra.Command{
	Use:   "inspect <image.iso>",
	Short: "Validate and inspect a Windows installation ISO",
	Long: "Parse the ISO9660, El Torito, and UDF structures of a Windows install ISO " +
		"and report correctness problems — most importantly UDF allocation descriptors " +
		"that overflow the short_ad limit, which make a >1 GiB boot.wim/install.wim " +
		"unreadable by the Windows boot manager. Exits non-zero when error-level issues " +
		"are found, so it can gate a build.",
	Args: cobra.ExactArgs(1),
	RunE: runISOInspect,
}

var isoExtractEFICmd = &cobra.Command{
	Use:   "extract-efi <image.iso> <out.img>",
	Short: "Extract the El Torito EFI boot image (efisys.bin) from an ISO",
	Args:  cobra.ExactArgs(2),
	RunE:  runISOExtractEFI,
}

var isoFixElToritoCmd = &cobra.Command{
	Use:   "fix-eltorito <image.iso>",
	Short: "Rewrite the El Torito catalog to UEFI-only (in place)",
	Long: "Promote the UEFI boot image to the validation/default entry (platform 0xEF) " +
		"and drop any x86 BIOS entry, matching Microsoft's ARM64 media and removing the " +
		"'Image type X64 can't be loaded' firmware error.",
	Args: cobra.ExactArgs(1),
	RunE: runISOFixElTorito,
}

func init() {
	isoInspectCmd.Flags().BoolP("quiet", "q", false, "print issues only, not the full structure")
	isoCmd.AddCommand(isoInspectCmd)
	isoCmd.AddCommand(isoExtractEFICmd)
	isoCmd.AddCommand(isoFixElToritoCmd)
}

func runISOInspect(cmd *cobra.Command, args []string) error {
	rep, err := isoinspect.Inspect(args[0])
	if err != nil {
		return err
	}
	if quiet, _ := cmd.Flags().GetBool("quiet"); quiet {
		for _, is := range rep.Issues {
			fmt.Println(is)
		}
	} else {
		fmt.Print(rep.Summary())
	}
	if !rep.OK() {
		return fmt.Errorf("%d validation error(s) found", len(rep.Errors()))
	}
	return nil
}

func runISOExtractEFI(cmd *cobra.Command, args []string) error {
	if err := isoinspect.ExtractElToritoEFIImage(args[0], args[1]); err != nil {
		return err
	}
	fmt.Printf("extracted El Torito EFI image to %s\n", args[1])
	return nil
}

func runISOFixElTorito(cmd *cobra.Command, args []string) error {
	if err := isoinspect.SetElToritoUEFIOnly(args[0]); err != nil {
		return err
	}
	fmt.Println("rewrote El Torito catalog to UEFI-only")
	return nil
}
