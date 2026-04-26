package feed

import "github.com/deploymenttheory/go-sdk-windowsuup/catalog"

// Event is a single Server-Sent Event received from the live feed stream.
type Event struct {
	Event string
	Data  string
}

// listResponse is the JSON envelope returned by GET /v1/feed.
type listResponse struct {
	Data []catalog.FeedEntry `json:"data"`
	Meta struct {
		Total  int64 `json:"total"`
		Limit  int   `json:"limit"`
		Offset int   `json:"offset"`
	} `json:"meta"`
}
