package files

import (
	"context"

	"github.com/deploymenttheory/go-sdk-windowsuup/winupdate"
)

// FileQuery is a fluent builder for filtering file results client-side.
// Construct one via Files.Query(uuid) and chain filter methods before
// calling Execute.
type FileQuery struct {
	svc      *Files
	uuid     string
	withURLs bool
	revision int
	filters  []func(winupdate.FileResult) bool
}

// WithURLs enables live CDN URL resolution for the given revision number.
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

// Where applies a custom predicate filter.
func (q *FileQuery) Where(fn func(f winupdate.FileResult) bool) *FileQuery {
	q.filters = append(q.filters, fn)
	return q
}

// Execute fetches files and applies all registered filters, returning only
// files that pass every filter.
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
