// Package builds provides Windows Update build discovery operations.
package builds

import (
	"context"
	"fmt"
	"strings"

	"github.com/deploymenttheory/go-sdk-windowsuup/internal/wuproto"
	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/client"
	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/constants"
	"github.com/deploymenttheory/go-sdk-windowsuup/windowsuup/shared/models"
	"resty.dev/v3"
)

// Service provides Windows Update build discovery operations.
type Service struct {
	client client.Client
}

// New returns a Service backed by the given client transport.
func New(c client.Client) *Service {
	return &Service{client: c}
}

// FetchBuilds queries the Windows Update service for available builds matching
// the given options. Each Build includes file metadata (SHA1, SHA256, size)
// but not CDN download URLs — call Files.GetFiles with WithCDNURLs for those.
func (s *Service) FetchBuilds(ctx context.Context, opts ...FetchOption) ([]models.Build, *resty.Response, error) {
	cfg := defaultFetchConfig()
	for _, o := range opts {
		o(cfg)
	}

	results, resp, err := s.client.FetchUpdates(ctx, wuproto.FetchRequest{
		Arch:       wuproto.Arch(cfg.arch),
		Ring:       wuproto.Ring(cfg.ring),
		Flight:     wuproto.Flight(cfg.flight),
		Build:      cfg.build,
		CheckBuild: cfg.checkBuild,
		SKU:        cfg.sku.ID,
	})
	if err != nil {
		return nil, resp, fmt.Errorf("FetchBuilds: %w", err)
	}

	builds := make([]models.Build, 0, len(results))
	for _, r := range results {
		b := models.Build{
			UUID:         r.UpdateID,
			Revision:     r.Revision,
			Title:        r.Title,
			Build:        r.Build,
			Arch:         constants.Arch(r.Arch),
			Ring:         cfg.ring,
			Flight:       cfg.flight,
			SKU:          cfg.sku.ID,
			IsInsider:    isInsiderRing(cfg.ring),
			IsStable:     isStableBuild(r.Build),
			DiscoveredAt: r.DiscoveredAt,
		}
		builds = append(builds, b)
	}
	return builds, resp, nil
}

// isInsiderRing returns true for non-Retail rings.
func isInsiderRing(ring constants.Ring) bool {
	return ring != constants.RingRetail
}

// isStableBuild returns true when the build string looks like a released
// (non-prerelease) Windows build (major 19041+, minor > 0).
func isStableBuild(build string) bool {
	parts := strings.Split(build, ".")
	if len(parts) < 4 {
		return false
	}
	var major, minor int
	fmt.Sscanf(parts[2], "%d", &major)
	fmt.Sscanf(parts[3], "%d", &minor)
	return major >= 19041 && minor > 0
}
