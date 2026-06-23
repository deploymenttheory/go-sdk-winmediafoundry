package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	esdapi "github.com/deploymenttheory/go-sdk-winmediafoundry/esd/api/esd"
)

var esdCmd = &cobra.Command{
	Use:   "esd",
	Short: "Media Creation Tool ESD catalog operations",
}

var esdCatalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "List the Media Creation Tool ESD catalog",
	Long:  "Fetch and parse Microsoft's products.cab and list the installation ESDs.",
	RunE:  runESDCatalog,
}

func init() {
	esdCatalogCmd.Flags().String("product", "windows11", "catalog to fetch (windows11 or windows10)")
	esdCatalogCmd.Flags().String("architecture", "", "filter by ESD architecture, e.g. x64 or ARM64")
	esdCatalogCmd.Flags().String("edition", "", "filter by edition, e.g. Professional")
	esdCatalogCmd.Flags().String("language", "", "filter by language code, e.g. en-us")
	esdCmd.AddCommand(esdCatalogCmd)
	rootCmd.AddCommand(esdCmd)
}

func runESDCatalog(cmd *cobra.Command, _ []string) error {
	c, err := newESDClient()
	if err != nil {
		return err
	}

	product := esdapi.Windows11
	switch strings.ToLower(mustString(cmd, "product")) {
	case "windows11", "win11", "11":
		product = esdapi.Windows11
	case "windows10", "win10", "10":
		product = esdapi.Windows10
	default:
		return fmt.Errorf("invalid --product %q (want windows11 or windows10)", mustString(cmd, "product"))
	}

	cat, _, err := c.Catalog(cmd.Context(), esdapi.WithProduct(product))
	if err != nil {
		return fmt.Errorf("fetch catalog: %w", err)
	}

	edition := mustString(cmd, "edition")
	arch := mustString(cmd, "architecture")
	lang := mustString(cmd, "language")
	imgs := cat.Filter(edition, arch, lang)

	fmt.Printf("%s: %d images (%d shown after filters)\n", product.Name, len(cat.Images), len(imgs))
	w := newTable()
	fmt.Fprintln(w, "EDITION\tARCH\tLANG\tSIZE (GB)\tSHA1")
	for _, img := range imgs {
		fmt.Fprintf(w, "%s\t%s\t%s\t%.2f\t%s\n",
			img.Edition, img.Architecture, img.LanguageCode, float64(img.SizeBytes)/1e9, img.SHA1)
	}
	return w.Flush()
}

func mustString(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}
