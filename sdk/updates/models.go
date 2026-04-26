package updates

import "github.com/deploymenttheory/go-sdk-windowsuup/winupdate"

// Request is the input to Updates.Fetch.
type Request struct {
	Arch   string `json:"arch"`
	Ring   string `json:"ring"`
	Flight string `json:"flight"`
	Build  string `json:"build,omitempty"`
	// CheckBuild is the OS version the device claims to be running. Set to an
	// old build (e.g. "10.0.16251.0") to receive the current stable Windows 11
	// release as an upgrade offer. Defaults to "10.0.16251.0" server-side when
	// empty.
	CheckBuild string `json:"check_build,omitempty"`
	SKU        int    `json:"sku,omitempty"`
}

// fetchResultResponse is the JSON envelope returned by POST /v1/updates/fetch.
type fetchResultResponse struct {
	Data winupdate.FetchAndStoreResult `json:"data"`
}
