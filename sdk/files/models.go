package files

import "github.com/deploymenttheory/go-sdk-windowsuup/winupdate"

// listResponse is the JSON envelope returned by GET /v1/builds/{uuid}/files.
type listResponse struct {
	Data []winupdate.FileResult `json:"data"`
}
