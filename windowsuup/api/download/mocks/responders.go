// Package mocks provides pre-configured GenericMock instances for download
// service unit tests.
package mocks

import (
	"net/http"

	windowsuupmocks "github.com/deploymenttheory/winmediafoundry/windowsuup/mocks"
)

const TestCDNURL = "https://download.windowsupdate.com/test/file.esd"

// NewDownloadSuccess returns a mock that returns a 200 OK response for a CDN
// GET request, with the given body as the file content.
func NewDownloadSuccess(body []byte) *windowsuupmocks.GenericMock {
	m := windowsuupmocks.NewGenericMock()
	m.Register("GET", TestCDNURL, http.StatusOK, body)
	return m
}

// NewDownloadHTTPError returns a mock that returns a 403 Forbidden error.
func NewDownloadHTTPError() *windowsuupmocks.GenericMock {
	m := windowsuupmocks.NewGenericMock()
	m.RegisterError("GET", TestCDNURL, http.StatusForbidden, "CDN 403 Forbidden")
	return m
}
