package sdk

import (
	"context"
	"fmt"
	"net/http"

	"github.com/deploymenttheory/go-sdk-windowsuup/catalog"
	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/transport"
)

// DiffService provides the GET /v1/diff endpoint.
type DiffService struct {
	t *transport.Transport
}

type diffResponse struct {
	Data catalog.BuildDiff `json:"data"`
}

// Compare returns the file-level diff between two builds identified by UUID.
func (s *DiffService) Compare(ctx context.Context, baseUUID, targetUUID string) (*catalog.BuildDiff, error) {
	var result diffResponse
	resp, err := s.t.Request(ctx).
		SetQueryParam("base", baseUUID).
		SetQueryParam("target", targetUUID).
		SetResult(&result).
		Get("/v1/diff")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("diff compare: HTTP %d", resp.StatusCode())
	}
	return &result.Data, nil
}
