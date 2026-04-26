// Package updates provides methods for the /v1/updates API endpoints.
package updates

import (
	"context"
	"fmt"
	"net/http"

	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/constants"
	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/transport"
	"github.com/deploymenttheory/go-sdk-windowsuup/winupdate"
)

// Updates provides methods for the /v1/updates endpoints.
type Updates struct {
	t *transport.Transport
}

// New returns a new Updates service backed by the given transport.
func New(t *transport.Transport) *Updates {
	return &Updates{t: t}
}

// Fetch triggers a live Windows Update SOAP query on the server, storing any
// newly discovered or updated builds in the catalog.
func (u *Updates) Fetch(ctx context.Context, req Request) (*winupdate.FetchAndStoreResult, error) {
	var result fetchResultResponse
	resp, err := u.t.Request(ctx).SetBody(req).SetResult(&result).Post(constants.EndpointUpdates)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("updates fetch: HTTP %d", resp.StatusCode())
	}
	return &result.Data, nil
}
