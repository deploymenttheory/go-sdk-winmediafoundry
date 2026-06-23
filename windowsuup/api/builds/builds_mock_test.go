package builds_test

import (
	"context"
	"errors"
	"testing"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/api/builds"
	buildsmocks "github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/api/builds/mocks"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/constants"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnit_Builds_FetchBuilds_HappyPath(t *testing.T) {
	mock := buildsmocks.NewFetchBuildsSuccess()
	svc := builds.New(mock)

	result, resp, err := svc.FetchBuilds(context.Background())

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, result, 1)
	assert.Equal(t, "test-uuid-1234-5678-abcd-ef0123456789", result[0].UUID)
	assert.Equal(t, 200, result[0].Revision)
	assert.Equal(t, "10.0.26120.4061", result[0].Build)
	assert.Equal(t, constants.ArchAMD64, result[0].Arch)
	assert.Equal(t, constants.RingRetail, result[0].Ring)
	assert.False(t, result[0].IsInsider)
	assert.NotZero(t, result[0].DiscoveredAt)
}

func TestUnit_Builds_FetchBuilds_WithBuild_SetsRingAndArch(t *testing.T) {
	mock := buildsmocks.NewFetchBuildsSuccess()
	svc := builds.New(mock)

	result, _, err := svc.FetchBuilds(context.Background(),
		builds.WithArch(constants.ArchARM64),
		builds.WithRing(constants.RingBeta),
	)

	require.NoError(t, err)
	// Ring and Arch are set from the option config, not from the SOAP response arch.
	require.Len(t, result, 1)
	assert.Equal(t, constants.RingBeta, result[0].Ring)
	assert.True(t, result[0].IsInsider)
}

func TestUnit_Builds_FetchBuilds_CookieError(t *testing.T) {
	m := mocks.NewGenericMock()
	m.SetCookieError(errors.New("cookie service unavailable"))
	svc := builds.New(m)

	result, resp, err := svc.FetchBuilds(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "acquire WU cookie")
	assert.Nil(t, result)
	assert.Nil(t, resp)
}

func TestUnit_Builds_FetchBuilds_SOAPError(t *testing.T) {
	mock := buildsmocks.NewFetchBuildsSOAPError()
	svc := builds.New(mock)

	result, _, err := svc.FetchBuilds(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "FetchBuilds")
	assert.Nil(t, result)
}

func TestUnit_Builds_FetchBuilds_NoRegisteredResponse(t *testing.T) {
	m := mocks.NewGenericMock()
	m.SetCookie("data", "expiry", "token")
	// No response registered → mock returns "no response registered" error.
	svc := builds.New(m)

	_, _, err := svc.FetchBuilds(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "FetchBuilds")
}
