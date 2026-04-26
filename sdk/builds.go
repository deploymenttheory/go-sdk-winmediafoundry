package sdk

import (
	"context"
	"fmt"
	"net/http"

	"github.com/deploymenttheory/go-sdk-windowsuup/catalog"
	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/transport"
)

// BuildsService provides methods for the /v1/builds endpoints.
type BuildsService struct {
	t *transport.Transport
}

// listBuildsResponse is the JSON response shape for GET /v1/builds.
type listBuildsResponse struct {
	Data []catalog.Build `json:"data"`
	Meta struct {
		Total  int64 `json:"total"`
		Limit  int   `json:"limit"`
		Offset int   `json:"offset"`
	} `json:"meta"`
}

type getBuildResponse struct {
	Data catalog.Build `json:"data"`
}

// List retrieves builds from the catalog with optional filtering and pagination.
func (s *BuildsService) List(ctx context.Context, q catalog.BuildQuery) ([]catalog.Build, int64, error) {
	req := s.t.Request(ctx).
		SetQueryParam("search", q.Search).
		SetQueryParam("arch", q.Arch).
		SetQueryParam("ring", q.Ring).
		SetQueryParam("limit", fmt.Sprintf("%d", q.Limit)).
		SetQueryParam("offset", fmt.Sprintf("%d", q.Offset))

	if q.StableOnly {
		req = req.SetQueryParam("stable", "true")
	}
	if q.OrderBy != "" {
		req = req.SetQueryParam("order", q.OrderBy)
	}

	req = req.SetResult(&listBuildsResponse{})
	resp, err := req.Get("/v1/builds")
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, 0, fmt.Errorf("builds list: HTTP %d", resp.StatusCode())
	}

	result := resp.Result().(*listBuildsResponse)
	return result.Data, result.Meta.Total, nil
}

// Get retrieves a single build by UUID.
func (s *BuildsService) Get(ctx context.Context, uuid string) (*catalog.Build, error) {
	var result getBuildResponse
	resp, err := s.t.Request(ctx).SetResult(&result).Get("/v1/builds/" + uuid)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil, fmt.Errorf("build %s not found", uuid)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("build get: HTTP %d", resp.StatusCode())
	}
	return &result.Data, nil
}
