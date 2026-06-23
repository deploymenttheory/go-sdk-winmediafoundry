package builds

import (
	"github.com/deploymenttheory/winmediafoundry/windowsuup/constants"
)

// FetchOption configures a FetchBuilds call.
type FetchOption func(*fetchConfig)

type fetchConfig struct {
	arch       constants.Arch
	ring       constants.Ring
	sku        constants.SKU
	checkBuild string
	flight     string
	build      string
}

func defaultFetchConfig() *fetchConfig {
	return &fetchConfig{
		arch:   constants.ArchAMD64,
		ring:   constants.RingRetail,
		sku:    constants.SKUPro,
		flight: "Active",
	}
}

// WithArch filters results to the given CPU architecture (default: ArchAMD64).
func WithArch(arch constants.Arch) FetchOption {
	return func(c *fetchConfig) { c.arch = arch }
}

// WithRing selects the Windows Update release channel (default: RingRetail).
func WithRing(ring constants.Ring) FetchOption {
	return func(c *fetchConfig) { c.ring = ring }
}

// WithSKU sets the Windows edition SKU (default: SKUPro / ID 48).
// The SKU affects which product category WU uses when selecting updates.
func WithSKU(sku constants.SKU) FetchOption {
	return func(c *fetchConfig) { c.sku = sku }
}

// WithCheckBuild overrides the OS version the client claims to be running.
// By default the client sends an old build string to trigger WU to offer the
// current release as an upgrade.
func WithCheckBuild(build string) FetchOption {
	return func(c *fetchConfig) { c.checkBuild = build }
}

// WithFlight sets the WU flight sub-channel (default: "Active").
func WithFlight(flight string) FetchOption {
	return func(c *fetchConfig) { c.flight = flight }
}

// WithBuild filters to a specific build version string (e.g. "26100.4061").
// When set, SyncUpdates is called with SyncCurrentVersionOnly=true.
func WithBuild(build string) FetchOption {
	return func(c *fetchConfig) { c.build = build }
}
