package files_test

import (
	"context"
	"errors"
	"testing"

	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/api/files"
	filesmocks "github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/api/files/mocks"
	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/constants"
	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/mocks"
	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/shared/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testBuild is a representative Build used across files tests.
var testBuild = models.Build{
	UUID:     "test-uuid-1234-5678-abcd-ef0123456789",
	Revision: 200,
	Build:    "10.0.26120.4061",
	Arch:     constants.ArchAMD64,
	Ring:     constants.RingRetail,
	SKU:      48,
}

func TestUnit_Files_GetFiles_WithCDNURLs_HappyPath(t *testing.T) {
	mock := filesmocks.NewGetFilesWithURLsSuccess()
	svc := files.New(mock)

	result, resp, err := svc.GetFiles(context.Background(), testBuild, files.WithCDNURLs())

	require.NoError(t, err)
	require.NotNil(t, resp)
	// PSF files are excluded; 2 valid files remain (.esd and .cab).
	assert.Len(t, result, 2)
	for _, f := range result {
		assert.NotEmpty(t, f.Name)
		assert.NotEmpty(t, f.URL)
		assert.NotEmpty(t, f.FileType)
	}
}

func TestUnit_Files_GetFiles_WithCDNURLs_ExtensionFilter(t *testing.T) {
	mock := filesmocks.NewGetFilesWithURLsSuccess()
	svc := files.New(mock)

	result, _, err := svc.GetFiles(context.Background(), testBuild,
		files.WithCDNURLs(),
		files.WithExtension(".esd"),
	)

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "esd", result[0].FileType)
}

func TestUnit_Files_GetFiles_MetadataOnly_HappyPath(t *testing.T) {
	mock := filesmocks.NewGetFilesMetadataSuccess()
	svc := files.New(mock)

	result, resp, err := svc.GetFiles(context.Background(), testBuild)

	require.NoError(t, err)
	require.NotNil(t, resp)
	// The fixture has one leaf update that matches the testBuild UUID.
	// The ESD file is returned; EXPRESS cab is excluded by soap.shouldExclude.
	require.Len(t, result, 1)
	assert.Equal(t, "Windows11.0-26120.4061-amd64.esd", result[0].Name)
}

func TestUnit_Files_GetFiles_CookieError(t *testing.T) {
	m := mocks.NewGenericMock()
	m.SetCookieError(errors.New("cookie unavailable"))
	svc := files.New(m)

	result, resp, err := svc.GetFiles(context.Background(), testBuild, files.WithCDNURLs())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "acquire WU cookie")
	assert.Nil(t, result)
	assert.Nil(t, resp)
}

func TestUnit_Files_GetFiles_SOAPError(t *testing.T) {
	mock := filesmocks.NewGetFilesSOAPError()
	svc := files.New(mock)

	result, _, err := svc.GetFiles(context.Background(), testBuild, files.WithCDNURLs())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "GetFiles")
	assert.Nil(t, result)
}
