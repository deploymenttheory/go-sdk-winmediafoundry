// Package mocks provides pre-configured GenericMock instances for files
// service unit tests.
package mocks

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/deploymenttheory/winmediafoundry/pkg/wuproto/soap"
	windowsuupmocks "github.com/deploymenttheory/winmediafoundry/windowsuup/mocks"
)

func fixturesDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "testdata", "soap")
}

func mustReadFixture(name string) []byte {
	data, err := os.ReadFile(filepath.Join(fixturesDir(), name))
	if err != nil {
		panic("files/mocks: failed to read fixture " + name + ": " + err.Error())
	}
	return data
}

// NewGetFilesWithURLsSuccess returns a mock pre-loaded with a valid
// GetExtendedUpdateInfo2 response containing CDN file URLs.
func NewGetFilesWithURLsSuccess() *windowsuupmocks.GenericMock {
	m := windowsuupmocks.NewGenericMock()
	m.SetCookie("test-enc-data", "2099-01-01T00:00:00Z", "test-device-token")
	m.Register("POST", soap.ClientSecuredEndpoint, http.StatusOK, mustReadFixture("get_extended_info_response.xml"))
	return m
}

// NewGetFilesMetadataSuccess returns a mock pre-loaded with a valid
// SyncUpdates response (used when WithCDNURLs is not set).
func NewGetFilesMetadataSuccess() *windowsuupmocks.GenericMock {
	m := windowsuupmocks.NewGenericMock()
	m.SetCookie("test-enc-data", "2099-01-01T00:00:00Z", "test-device-token")
	m.Register("POST", soap.ClientEndpoint, http.StatusOK, mustReadFixture("fetch_updates_response.xml"))
	return m
}

// NewGetFilesSOAPError returns a mock that returns a 500 error for EUI2 calls.
func NewGetFilesSOAPError() *windowsuupmocks.GenericMock {
	m := windowsuupmocks.NewGenericMock()
	m.SetCookie("test-enc-data", "2099-01-01T00:00:00Z", "test-device-token")
	m.RegisterError("POST", soap.ClientSecuredEndpoint, http.StatusInternalServerError, "SOAP EUI2 error")
	return m
}
