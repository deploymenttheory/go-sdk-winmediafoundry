package sdk

import (
	"context"
	"fmt"
	"net/http"

	"github.com/deploymenttheory/go-sdk-uupdump/sdk/transport"
	"github.com/deploymenttheory/go-sdk-uupdump/winupdate"
)

// UpdatesService provides the POST /v1/updates/fetch endpoint.
type UpdatesService struct {
	t *transport.Transport
}

type fetchResultResponse struct {
	Data winupdate.FetchAndStoreResult `json:"data"`
}

// FetchRequest is the input to UpdatesService.Fetch.
type FetchRequest struct {
	Arch   string `json:"arch"`
	Ring   string `json:"ring"`
	Flight string `json:"flight"`
	Build  string `json:"build,omitempty"`
	SKU    int    `json:"sku,omitempty"`
}

// Fetch triggers a live Windows Update SOAP query on the server.
func (s *UpdatesService) Fetch(ctx context.Context, req FetchRequest) (*winupdate.FetchAndStoreResult, error) {
	var result fetchResultResponse
	resp, err := s.t.Request(ctx).SetBody(req).SetResult(&result).Post("/v1/updates/fetch")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("updates fetch: HTTP %d", resp.StatusCode())
	}
	return &result.Data, nil
}
