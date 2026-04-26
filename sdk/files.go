package sdk

import (
	"context"
	"fmt"
	"net/http"

	"github.com/deploymenttheory/go-sdk-windowsuup/sdk/transport"
	"github.com/deploymenttheory/go-sdk-windowsuup/winupdate"
)

// FilesService provides methods for the /v1/builds/{uuid}/files endpoints.
type FilesService struct {
	t *transport.Transport
}

type listFilesResponse struct {
	Data []winupdate.FileResult `json:"data"`
}

// List retrieves file metadata for a build. Set withURLs=true to resolve
// live CDN download URLs (requires revision to be set).
func (s *FilesService) List(ctx context.Context, uuid string, withURLs bool, revision int) ([]winupdate.FileResult, error) {
	req := s.t.Request(ctx).SetResult(&listFilesResponse{})
	if withURLs {
		req = req.
			SetQueryParam("with_urls", "true").
			SetQueryParam("revision", fmt.Sprintf("%d", revision))
	}

	resp, err := req.Get("/v1/builds/" + uuid + "/files")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("files list: HTTP %d", resp.StatusCode())
	}
	return resp.Result().(*listFilesResponse).Data, nil
}

// FileQuery is a fluent builder for filtering file results client-side.
type FileQuery struct {
	svc      *FilesService
	uuid     string
	withURLs bool
	revision int
	filters  []func(winupdate.FileResult) bool
}

// QueryFiles returns a FileQuery builder for the given build UUID.
func (s *FilesService) QueryFiles(uuid string) *FileQuery {
	return &FileQuery{svc: s, uuid: uuid}
}

// WithURLs enables live CDN URL resolution.
func (q *FileQuery) WithURLs(revision int) *FileQuery {
	q.withURLs = true
	q.revision = revision
	return q
}

// ByExtension filters files by extension (e.g. ".esd").
func (q *FileQuery) ByExtension(ext string) *FileQuery {
	q.filters = append(q.filters, func(f winupdate.FileResult) bool {
		n := f.Name
		if len(n) < len(ext) {
			return false
		}
		return n[len(n)-len(ext):] == ext
	})
	return q
}

// ByName filters files whose name contains substr.
func (q *FileQuery) ByName(substr string) *FileQuery {
	q.filters = append(q.filters, func(f winupdate.FileResult) bool {
		return containsStr(f.Name, substr)
	})
	return q
}

// LargerThan filters files larger than minBytes.
func (q *FileQuery) LargerThan(minBytes int64) *FileQuery {
	q.filters = append(q.filters, func(f winupdate.FileResult) bool {
		return f.SizeBytes > minBytes
	})
	return q
}

// SmallerThan filters files smaller than maxBytes.
func (q *FileQuery) SmallerThan(maxBytes int64) *FileQuery {
	q.filters = append(q.filters, func(f winupdate.FileResult) bool {
		return f.SizeBytes < maxBytes
	})
	return q
}

// Where applies a custom predicate.
func (q *FileQuery) Where(fn func(f winupdate.FileResult) bool) *FileQuery {
	q.filters = append(q.filters, fn)
	return q
}

// Execute fetches files and applies all registered filters.
func (q *FileQuery) Execute(ctx context.Context) ([]winupdate.FileResult, error) {
	files, err := q.svc.List(ctx, q.uuid, q.withURLs, q.revision)
	if err != nil {
		return nil, err
	}
	var out []winupdate.FileResult
	for _, f := range files {
		pass := true
		for _, filter := range q.filters {
			if !filter(f) {
				pass = false
				break
			}
		}
		if pass {
			out = append(out, f)
		}
	}
	return out, nil
}

func containsStr(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
