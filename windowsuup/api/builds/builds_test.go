package builds

import (
	"testing"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/constants"
	"github.com/stretchr/testify/assert"
)

func TestIsStableBuild(t *testing.T) {
	stable := []string{
		"10.0.19041.1",   // Win10 20H1 GA
		"10.0.22621.1",   // Win11 22H2 GA
		"10.0.26100.1",   // Win11 24H2 GA
		"10.0.26100.100",
	}
	for _, b := range stable {
		t.Run(b, func(t *testing.T) {
			assert.True(t, isStableBuild(b), "expected %q to be stable", b)
		})
	}

	unstable := []string{
		"10.0.9600.0",  // very old / Insider trigger build
		"10.0.26120.0", // Insider Preview (minor == 0)
		"10.0.26120",   // too few parts
		"garbage",
		"",
	}
	for _, b := range unstable {
		t.Run(b, func(t *testing.T) {
			assert.False(t, isStableBuild(b), "expected %q to be unstable", b)
		})
	}
}

func TestIsInsiderRing(t *testing.T) {
	assert.False(t, isInsiderRing(constants.RingRetail))
	assert.True(t, isInsiderRing(constants.RingExperimental))
	assert.True(t, isInsiderRing(constants.RingBeta))
	assert.True(t, isInsiderRing(constants.RingReleasePreview))
	assert.True(t, isInsiderRing(constants.RingCanary))
	assert.True(t, isInsiderRing(constants.RingMSIT))
}
