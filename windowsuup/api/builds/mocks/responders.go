// Package mocks provides pre-configured GenericMock instances for builds
// service unit tests.
package mocks

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/deploymenttheory/winmediafoundry/internal/wuproto/soap"
	windowsuupmocks "github.com/deploymenttheory/winmediafoundry/windowsuup/mocks"
)

// fixturesDir returns the path to the shared testdata/soap directory relative
// to this file's location in the source tree.
func fixturesDir() string {
	_, file, _, _ := runtime.Caller(0)
	// this file: windowsuup/api/builds/mocks/responders.go
	// testdata:  <repo_root>/testdata/soap/
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "testdata", "soap")
}

func mustReadFixture(name string) []byte {
	data, err := os.ReadFile(filepath.Join(fixturesDir(), name))
	if err != nil {
		panic("builds/mocks: failed to read fixture " + name + ": " + err.Error())
	}
	return data
}

// NewFetchBuildsSuccess returns a mock pre-loaded with a valid SyncUpdates
// response containing one leaf build.
func NewFetchBuildsSuccess() *windowsuupmocks.GenericMock {
	m := windowsuupmocks.NewGenericMock()
	m.SetCookie("test-enc-data", "2099-01-01T00:00:00Z", "test-device-token")
	m.Register("POST", soap.ClientEndpoint, http.StatusOK, mustReadFixture("fetch_updates_response.xml"))
	return m
}

// NewFetchBuildsSOAPError returns a mock that returns a 500 SOAP error response.
func NewFetchBuildsSOAPError() *windowsuupmocks.GenericMock {
	m := windowsuupmocks.NewGenericMock()
	m.SetCookie("test-enc-data", "2099-01-01T00:00:00Z", "test-device-token")
	m.RegisterError("POST", soap.ClientEndpoint, http.StatusInternalServerError, "SOAP 500 error")
	return m
}
