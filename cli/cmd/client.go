package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/esd"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/softwaredownload"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup"
	buildsapi "github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/api/builds"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/constants"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/shared/models"
)

// newLogger builds a console zap logger at the configured level, with timestamps
// dropped for cleaner CLI output.
func newLogger() (*zap.Logger, error) {
	lvl, err := zapcore.ParseLevel(viper.GetString("log-level"))
	if err != nil {
		lvl = zapcore.WarnLevel
	}
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(lvl)
	cfg.Encoding = "console"
	cfg.EncoderConfig.TimeKey = ""
	return cfg.Build()
}

// newWUClient builds a Windows Update service client from Viper settings.
func newWUClient() (*windowsuup.Client, error) {
	logger, err := newLogger()
	if err != nil {
		return nil, err
	}
	return windowsuup.NewClient(
		windowsuup.WithTimeout(viper.GetDuration("timeout")),
		windowsuup.WithLogger(logger),
	)
}

// newESDClient builds an ESD catalog client from Viper settings.
func newESDClient() (*esd.Client, error) {
	logger, err := newLogger()
	if err != nil {
		return nil, err
	}
	return esd.NewClient(
		esd.WithTimeout(viper.GetDuration("timeout")),
		esd.WithLogger(logger),
	)
}

// newSWDLClient builds a consumer software-download client from Viper settings.
func newSWDLClient() (*softwaredownload.Client, error) {
	logger, err := newLogger()
	if err != nil {
		return nil, err
	}
	return softwaredownload.NewClient(
		softwaredownload.WithTimeout(viper.GetDuration("timeout")),
		softwaredownload.WithLogger(logger),
	)
}

// skusByName maps friendly SKU names to constants.
var skusByName = map[string]constants.SKU{
	"home":       constants.SKUHome,
	"pro":        constants.SKUPro,
	"enterprise": constants.SKUEnterprise,
	"education":  constants.SKUEducation,
}

// wuFetchOptions builds the FetchBuilds options shared by the Windows Update
// commands from the global arch/ring/sku settings, plus an optional build filter.
func wuFetchOptions(buildFilter string) ([]buildsapi.FetchOption, error) {
	arch := constants.Arch(strings.ToLower(viper.GetString("arch")))
	switch arch {
	case constants.ArchAMD64, constants.ArchX86, constants.ArchARM64:
	default:
		return nil, fmt.Errorf("invalid --arch %q (want amd64, x86, or arm64)", arch)
	}

	sku, ok := skusByName[strings.ToLower(viper.GetString("sku"))]
	if !ok {
		return nil, fmt.Errorf("invalid --sku %q (want home, pro, enterprise, or education)", viper.GetString("sku"))
	}

	opts := []buildsapi.FetchOption{
		buildsapi.WithArch(arch),
		buildsapi.WithRing(constants.Ring(viper.GetString("ring"))),
		buildsapi.WithSKU(sku),
	}
	if buildFilter != "" {
		opts = append(opts, buildsapi.WithBuild(buildFilter))
	}
	return opts, nil
}

// resolveBuild fetches builds matching the current filters and returns the first
// one, erroring if none match.
func resolveBuild(ctx context.Context, c *windowsuup.Client, buildFilter string) (models.Build, error) {
	opts, err := wuFetchOptions(buildFilter)
	if err != nil {
		return models.Build{}, err
	}
	builds, _, err := c.Builds.FetchBuilds(ctx, opts...)
	if err != nil {
		return models.Build{}, fmt.Errorf("fetch builds: %w", err)
	}
	if len(builds) == 0 {
		return models.Build{}, fmt.Errorf("no build found matching the given filters")
	}
	return builds[0], nil
}
