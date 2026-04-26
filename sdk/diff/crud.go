// Package diff provides methods for the /v1/diff API endpoint.
package diff

import (
	"context"
	"fmt"
	"net/http"

	"github.com/deploymenttheory/go-sdk-windowsuup/catalog"
	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/constants"
	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/transport"
)

// Diff provides methods for the /v1/diff endpoint.
type Diff struct {
	t *transport.Transport
}

// New returns a new Diff service backed by the given transport.
func New(t *transport.Transport) *Diff {
	return &Diff{t: t}
}

// Compare returns the file-level diff between two builds identified by UUID.
func (d *Diff) Compare(ctx context.Context, baseUUID, targetUUID string) (*catalog.BuildDiff, error) {
	var result diffResponse
	resp, err := d.t.Request(ctx).
		SetQueryParam("base", baseUUID).
		SetQueryParam("target", targetUUID).
		SetResult(&result).
		Get(constants.EndpointDiff)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("diff compare: HTTP %d", resp.StatusCode())
	}
	return &result.Data, nil
}
