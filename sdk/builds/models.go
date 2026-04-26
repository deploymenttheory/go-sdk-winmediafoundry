package builds

import "github.com/deploymenttheory/go-sdk-windowsuup/catalog"

// listResponse is the JSON envelope returned by GET /v1/builds.
type listResponse struct {
	Data []catalog.Build `json:"data"`
	Meta struct {
		Total  int64 `json:"total"`
		Limit  int   `json:"limit"`
		Offset int   `json:"offset"`
	} `json:"meta"`
}

// getResponse is the JSON envelope returned by GET /v1/builds/{uuid}.
type getResponse struct {
	Data catalog.Build `json:"data"`
}
