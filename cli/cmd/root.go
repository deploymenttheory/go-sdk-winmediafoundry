// Package cmd implements the winmediafoundry CLI commands (Cobra) with Viper
// configuration: flags, environment variables (WINMEDIAFOUNDRY_*), and an
// optional config file ($HOME/.winmediafoundry.yaml) all feed the same settings.
package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "winmediafoundry",
	Short: "Acquire and build Windows installation media",
	Long: `winmediafoundry is a pure-Go toolkit for acquiring and building Windows
installation media.

  builds / files / download / diff   discover and fetch from Windows Update
  esd catalog                        list the Media Creation Tool ESD catalog
  swdl list / resolve / download     download consumer Windows 11 ISOs
  wim info / tree / extract          inspect and extract WIM/ESD images
  iso build                          master a bootable ISO from an ESD

Configuration is layered (lowest to highest precedence): config file
($HOME/.winmediafoundry.yaml), environment (WINMEDIAFOUNDRY_*), then flags.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	pf := rootCmd.PersistentFlags()
	pf.StringVar(&cfgFile, "config", "", "config file (default $HOME/.winmediafoundry.yaml)")
	pf.Duration("timeout", 2*time.Minute, "HTTP request timeout for network commands")
	pf.String("log-level", "warn", "log verbosity (debug, info, warn, error)")
	pf.String("arch", "amd64", "CPU architecture for Windows Update commands (amd64, x86, arm64)")
	pf.String("ring", "Retail", "Windows Update release ring (Retail, Beta, ReleasePreview, Experimental, Canary)")
	pf.String("sku", "pro", "Windows SKU (home, pro, enterprise, education)")

	for _, k := range []string{"timeout", "log-level", "arch", "ring", "sku"} {
		_ = viper.BindPFlag(k, pf.Lookup(k))
	}
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else if home, err := os.UserHomeDir(); err == nil {
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".winmediafoundry")
	}
	viper.SetEnvPrefix("WINMEDIAFOUNDRY")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_")) // log-level -> WINMEDIAFOUNDRY_LOG_LEVEL
	viper.AutomaticEnv()
	_ = viper.ReadInConfig()
}
