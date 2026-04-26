// Package files provides methods for the /v1/builds/{uuid}/files API endpoints.
package files

import (
	"context"
	"fmt"
	"net/http"

	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/constants"
	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/transport"
	"github.com/deploymenttheory/go-sdk-windowsuup/winupdate"
)

// Files provides methods for the /v1/builds/{uuid}/files endpoints.
type Files struct {
	t *transport.Transport
}

// New returns a new Files service backed by the given transport.
func New(t *transport.Transport) *Files {
	return &Files{t: t}
}

// List retrieves file metadata for a build. Set withURLs=true to resolve
// live CDN download URLs (requires revision to be set).
func (f *Files) List(ctx context.Context, uuid string, withURLs bool, revision int) ([]winupdate.FileResult, error) {
	req := f.t.Request(ctx).SetResult(&listResponse{})
	if withURLs {
		req = req.
			SetQueryParam("with_urls", "true").
			SetQueryParam("revision", fmt.Sprintf("%d", revision))
	}

	resp, err := req.Get(constants.EndpointBuilds + "/" + uuid + "/files")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("files list: HTTP %d", resp.StatusCode())
	}
	return resp.Result().(*listResponse).Data, nil
}

// Query returns a FileQuery fluent builder for the given build UUID.
func (f *Files) Query(uuid string) *FileQuery {
	return &FileQuery{svc: f, uuid: uuid}
}
