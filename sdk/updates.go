package sdk

import (
	"context"
	"fmt"
	"net/http"

	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/transport"
	"github.com/deploymenttheory/go-sdk-windowsuup/winupdate"
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
	// CheckBuild is the OS version the device claims to be running. Set to an
	// old build (e.g. "10.0.16251.0") to receive the current stable Windows 11
	// release as an upgrade offer. Defaults to "10.0.16251.0" server-side when
	// empty.
	CheckBuild string `json:"check_build,omitempty"`
	SKU        int    `json:"sku,omitempty"`
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
