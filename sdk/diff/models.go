package diff

import "github.com/deploymenttheory/go-sdk-windowsuup/catalog"

// diffResponse is the JSON envelope returned by GET /v1/diff.
type diffResponse struct {
	Data catalog.BuildDiff `json:"data"`
}
