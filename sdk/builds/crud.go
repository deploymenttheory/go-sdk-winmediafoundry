// Package builds provides methods for the /v1/builds API endpoints.
package builds

import (
	"context"
	"fmt"
	"net/http"

	"github.com/deploymenttheory/go-sdk-windowsuup/catalog"
	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/constants"
	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/transport"
)

// Builds provides methods for the /v1/builds endpoints.
type Builds struct {
	t *transport.Transport
}

// New returns a new Builds service backed by the given transport.
func New(t *transport.Transport) *Builds {
	return &Builds{t: t}
}

// List retrieves builds from the catalog with optional filtering and pagination.
func (b *Builds) List(ctx context.Context, q catalog.BuildQuery) ([]catalog.Build, int64, error) {
	req := b.t.Request(ctx).
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
	if q.Desc {
		req = req.SetQueryParam("desc", "true")
	}

	req = req.SetResult(&listResponse{})
	resp, err := req.Get(constants.EndpointBuilds)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, 0, fmt.Errorf("builds list: HTTP %d", resp.StatusCode())
	}

	result := resp.Result().(*listResponse)
	return result.Data, result.Meta.Total, nil
}

// GetByID retrieves a single build by UUID.
func (b *Builds) GetByID(ctx context.Context, uuid string) (*catalog.Build, error) {
	var result getResponse
	resp, err := b.t.Request(ctx).SetResult(&result).Get(constants.EndpointBuilds + "/" + uuid)
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
