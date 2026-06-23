// Package builds provides Windows Update build discovery operations.
package builds

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/deploymenttheory/go-sdk-winmediafoundry/pkg/wuproto/soap"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/client"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/constants"
	"github.com/deploymenttheory/go-sdk-winmediafoundry/windowsuup/shared/models"
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

	arch := string(cfg.arch)
	ring := string(cfg.ring)

	// deviceBuild is the OS version sent in SOAP device and product attributes.
	//
	// Precedence:
	//   1. cfg.build — when querying a specific build, use it directly.
	//   2. cfg.checkBuild — caller-supplied override.
	//   3. soap.DefaultCheckBuild — ring-appropriate default:
	//        Retail → "10.0.26100.1" so Branch=ge_release is derived correctly.
	//        Insider/flight rings → "10.0.9600.0"; explicit FlightingBranchName overrides branch.
	deviceBuild := cfg.build
	if deviceBuild == "" {
		deviceBuild = cfg.checkBuild
	}
	if deviceBuild == "" {
		deviceBuild = soap.DefaultCheckBuild(string(cfg.ring))
	}

	syncCurrentOnly := cfg.build != ""

	deviceAttrs := soap.BuildDeviceAttributes(arch, ring, deviceBuild, "", cfg.sku.ID, "")
	products := soap.BuildProductsString(arch, ring, deviceBuild, cfg.sku.ID)

	// Cookie-aware retry: invalidate and retry once on cookie errors.
	const maxCookieRetries = 1
	for attempt := 0; attempt <= maxCookieRetries; attempt++ {
		encData, expiry, devToken, err := s.client.AcquireWUCookie(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("FetchBuilds: acquire WU cookie: %w", err)
		}

		envelope := soap.BuildSyncUpdatesEnvelope(
			time.Now(), devToken, encData, expiry,
			deviceAttrs, soap.CallerAttrs, products,
			syncCurrentOnly,
		)

		resp, err := s.client.NewRequest(ctx).
			SetHeader("Content-Type", constants.ApplicationSOAPXML).
			SetHeader("SOAPAction", soap.SyncUpdatesAction).
			SetBody(envelope).
			Post(soap.ClientEndpoint)
		if err != nil {
			if attempt < maxCookieRetries && soap.IsCookieError(err.Error()) {
				s.client.InvalidateWUCookie()
				continue
			}
			return nil, resp, fmt.Errorf("FetchBuilds: SOAP call: %w", err)
		}

		results, parseErr := soap.ParseSyncUpdatesResponse(resp.Bytes())
		if parseErr != nil {
			return nil, resp, fmt.Errorf("FetchBuilds: parse response: %w", parseErr)
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

	return nil, nil, fmt.Errorf("FetchBuilds: unexpected retry loop exit")
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
