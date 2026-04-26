// Package diff provides build file-set comparison operations.
package diff

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/api/files"
	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/client"
	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/shared/models"
	"resty.dev/v3"
)

// Service provides build file-set comparison operations.
type Service struct {
	client   client.Client
	filesSvc *files.Service
}

// New returns a Service backed by the given client transport.
func New(c client.Client) *Service {
	return &Service{
		client:   c,
		filesSvc: files.New(c),
	}
}

// Diff compares the file sets of two builds and returns what was added,
// removed, changed, or is unchanged between them.
//
// CDN URLs are not resolved — this is a pure metadata comparison.
// Files are matched by name and compared by SHA256 (falling back to SHA1,
// then size). The returned *resty.Response is from the final SOAP call made.
func (s *Service) Diff(ctx context.Context, buildA models.Build, buildB models.Build) (*models.BuildDiff, *resty.Response, error) {
	filesA, resp, err := s.filesSvc.GetFiles(ctx, buildA)
	if err != nil {
		return nil, resp, fmt.Errorf("Diff: fetch files for base build %s: %w", buildA.UUID, err)
	}

	filesB, resp, err := s.filesSvc.GetFiles(ctx, buildB)
	if err != nil {
		return nil, resp, fmt.Errorf("Diff: fetch files for target build %s: %w", buildB.UUID, err)
	}

	return diffFileSets(buildA, buildB, filesA, filesB), resp, nil
}

// diffFileSets performs the client-side comparison.
func diffFileSets(buildA, buildB models.Build, filesA, filesB []models.File) *models.BuildDiff {
	indexA := make(map[string]models.File, len(filesA))
	for _, f := range filesA {
		indexA[f.Name] = f
	}
	indexB := make(map[string]models.File, len(filesB))
	for _, f := range filesB {
		indexB[f.Name] = f
	}

	d := &models.BuildDiff{
		BaseUUID:    buildA.UUID,
		TargetUUID:  buildB.UUID,
		BaseBuild:   buildA.Build,
		TargetBuild: buildB.Build,
	}

	// Files in B: added or changed relative to A.
	for name, fb := range indexB {
		fa, inA := indexA[name]
		if !inA {
			d.Added = append(d.Added, fb)
			continue
		}
		if fileChanged(fa, fb) {
			d.Changed = append(d.Changed, models.FileDiff{
				Name:       name,
				BaseFile:   fa,
				TargetFile: fb,
			})
		} else {
			d.Unchanged++
		}
	}

	// Files in A but not B: removed.
	for name, fa := range indexA {
		if _, inB := indexB[name]; !inB {
			d.Removed = append(d.Removed, fa)
		}
	}

	return d
}

// fileChanged returns true if the two files differ in content.
// SHA256 is preferred; SHA1 is used as fallback; size as last resort.
func fileChanged(a, b models.File) bool {
	if a.SHA256 != "" && b.SHA256 != "" {
		return a.SHA256 != b.SHA256
	}
	if a.SHA1 != "" && b.SHA1 != "" {
		return a.SHA1 != b.SHA1
	}
	return a.SizeBytes != b.SizeBytes
}
