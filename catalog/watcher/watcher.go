// Package watcher provides a background poller that periodically queries the
// Windows Update service, writes new builds into the catalog, and emits
// lifecycle events via catalog.EventEmitter.
package watcher

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/deploymenttheory/go-sdk-windowsuup/catalog"
	"github.com/deploymenttheory/go-sdk-windowsuup/wuproto"
	"go.uber.org/zap"
)

// DefaultInterval is the polling cadence when none is supplied.
const DefaultInterval = 30 * time.Minute

// Watcher polls the Windows Update service on a configurable interval,
// writes discovered builds to a catalog.Store, and notifies a
// catalog.EventEmitter about new and updated builds.
//
// Create one with New and call Start to begin polling.
type Watcher struct {
	wu      wuproto.WindowsUpdateClient
	store   catalog.Store
	emitter catalog.EventEmitter
	logger  *zap.Logger

	// targets is the cross-product matrix of FetchRequests to issue per tick.
	targets  []wuproto.FetchRequest
	interval time.Duration

	stopCh chan struct{}
	doneCh chan struct{}
}

// New creates a Watcher. If interval is ≤ 0, DefaultInterval is used.
// If targets is empty the default scan matrix is used (amd64+arm64 × Dev/Beta/ReleasePreview/Retail).
func New(
	wu wuproto.WindowsUpdateClient,
	store catalog.Store,
	emitter catalog.EventEmitter,
	logger *zap.Logger,
	interval time.Duration,
	targets []wuproto.FetchRequest,
) *Watcher {
	if interval <= 0 {
		interval = DefaultInterval
	}
	if len(targets) == 0 {
		targets = defaultTargets()
	}
	return &Watcher{
		wu:       wu,
		store:    store,
		emitter:  emitter,
		logger:   logger,
		targets:  targets,
		interval: interval,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// Start begins the polling loop in a background goroutine. The loop runs until
// ctx is cancelled or Stop is called.
func (w *Watcher) Start(ctx context.Context) {
	go w.run(ctx)
}

// Stop signals the watcher to exit and waits for the current scan to finish.
func (w *Watcher) Stop() {
	close(w.stopCh)
	<-w.doneCh
}

func (w *Watcher) run(ctx context.Context) {
	defer close(w.doneCh)

	// Run one scan immediately, then tick.
	w.scan(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.scan(ctx)
		}
	}
}

// scan issues all configured FetchRequests and persists the results.
func (w *Watcher) scan(ctx context.Context) {
	w.logger.Info("watcher scan started", zap.Int("targets", len(w.targets)))
	for _, req := range w.targets {
		select {
		case <-ctx.Done():
			return
		default:
		}
		w.scanTarget(ctx, req)
	}
	w.logger.Info("watcher scan completed")
}

func (w *Watcher) scanTarget(ctx context.Context, req wuproto.FetchRequest) {
	results, err := w.wu.FetchUpdates(ctx, req)
	if err != nil {
		w.logger.Warn("FetchUpdates failed",
			zap.String("arch", string(req.Arch)),
			zap.String("ring", string(req.Ring)),
			zap.Error(err),
		)
		return
	}

	for _, r := range results {
		w.processResult(ctx, req, r)
	}
}

func (w *Watcher) processResult(ctx context.Context, req wuproto.FetchRequest, r wuproto.UpdateResult) {
	// Convert UpdateResult to catalog.Build with classification.
	b := ResultToBuild(r, req)

	isNew, err := w.store.UpsertBuild(ctx, b)
	if err != nil {
		w.logger.Error("UpsertBuild failed", zap.String("uuid", r.UpdateID), zap.Error(err))
		return
	}

	// Persist file metadata.
	if len(r.Files) > 0 {
		files := ResultFilesToCatalog(r)
		if err := w.store.UpsertFiles(ctx, b.UUID, files); err != nil {
			w.logger.Warn("UpsertFiles failed", zap.String("uuid", b.UUID), zap.Error(err))
		}
	}

	eventType := catalog.EventBuildUpdated
	if isNew {
		eventType = catalog.EventBuildDiscovered
	}

	entry := catalog.FeedEntry{
		EventType:   string(eventType),
		BuildUUID:   b.UUID,
		BuildTitle:  b.Title,
		BuildNumber: b.Build,
		Arch:        b.Arch,
		Ring:        b.Ring,
		OccurredAt:  time.Now().UTC(),
	}
	if err := w.store.AppendFeedEntry(ctx, entry); err != nil {
		w.logger.Warn("AppendFeedEntry failed", zap.String("uuid", b.UUID), zap.Error(err))
	}

	event := catalog.BuildEvent{
		Type:  eventType,
		Build: b,
	}
	w.emitter.Emit(ctx, event)

	if isNew {
		w.logger.Info("new build discovered",
			zap.String("uuid", b.UUID),
			zap.String("title", b.Title),
			zap.String("ring", b.Ring),
		)
	}
}

// ResultToBuild maps a wuproto.UpdateResult to a catalog.Build.
func ResultToBuild(r wuproto.UpdateResult, req wuproto.FetchRequest) catalog.Build {
	major, minor := parseBuildVersion(r.Build)
	return catalog.Build{
		UUID:         r.UpdateID,
		Revision:     r.Revision,
		Title:        r.Title,
		Build:        r.Build,
		MajorVersion: major,
		MinorVersion: minor,
		Arch:         string(r.Arch),
		Ring:         string(req.Ring),
		Flight:       string(req.Flight),
		IsStable:     isStableBuild(r.Title),
		IsInsider:    isInsiderBuild(r.Title),
		IsCumulative: isCumulativeBuild(r.Title),
		DiscoveredAt: r.DiscoveredAt,
	}
}

// ResultFilesToCatalog converts wuproto.FileMetadata to catalog.File values.
func ResultFilesToCatalog(r wuproto.UpdateResult) []catalog.File {
	files := make([]catalog.File, 0, len(r.Files))
	for _, fm := range r.Files {
		files = append(files, catalog.File{
			UUID:       r.UpdateID,
			Name:       fm.Name,
			SHA1:       fm.SHA1,
			SHA256:     fm.SHA256,
			SizeBytes:  fm.SizeBytes,
			FileType:   classifyFile(fm.Name),
			ModifiedAt: fm.Modified,
		})
	}
	return files
}

// defaultTargets returns the standard scan matrix:
// {amd64, arm64} × {Dev, Beta, ReleasePreview, Retail} × FlightActive.
func defaultTargets() []wuproto.FetchRequest {
	archs := []wuproto.Arch{wuproto.ArchAMD64, wuproto.ArchARM64}
	rings := []wuproto.Ring{wuproto.RingDev, wuproto.RingBeta, wuproto.RingReleasePreview, wuproto.RingRetail}

	var targets []wuproto.FetchRequest
	for _, arch := range archs {
		for _, ring := range rings {
			targets = append(targets, wuproto.FetchRequest{
				Arch:   arch,
				Ring:   ring,
				Flight: wuproto.FlightActive,
				SKU:    48, // Windows 11 Professional
				Type:   wuproto.BuildTypeProduction,
			})
		}
	}
	return targets
}

// ─── Build classification helpers ─────────────────────────────────────────

func isStableBuild(title string) bool {
	if isInsiderBuild(title) || isCumulativeBuild(title) {
		return false
	}
	// Must mention "Feature Update" or be a numbered OS release.
	return containsFold(title, "Feature Update") ||
		(containsFold(title, "Windows") && !containsFold(title, ".NET") &&
			!containsFold(title, "Preview") && !containsFold(title, "Insider"))
}

func isInsiderBuild(title string) bool {
	return containsFold(title, "Insider Preview") || containsFold(title, "Insider")
}

func isCumulativeBuild(title string) bool {
	return containsFold(title, "Cumulative Update") || containsFold(title, "-KB")
}

func containsFold(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(s) > 0 && containsFoldHelper(s, substr))
}

func containsFoldHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalFold(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca == cb {
			continue
		}
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// classifyFile returns the FileType for the given filename.
func classifyFile(name string) catalog.FileType {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".esd"):
		return catalog.FileTypeESD
	case strings.HasSuffix(lower, ".cab"):
		return catalog.FileTypeCAB
	case strings.HasSuffix(lower, ".psf"):
		return catalog.FileTypePSF
	case strings.HasSuffix(lower, ".msix"), strings.HasSuffix(lower, ".msixbundle"):
		return catalog.FileTypeMSIX
	case strings.Contains(lower, "_diff."):
		return catalog.FileTypeDifferential
	case strings.Contains(lower, "express"):
		return catalog.FileTypeEXPRESS
	default:
		return catalog.FileTypeUnknown
	}
}

// parseBuildVersion splits "10.0.26120.4061" into (26120, 4061).
func parseBuildVersion(build string) (major, minor int) {
	parts := strings.Split(build, ".")
	if len(parts) >= 4 {
		fmt.Sscanf(parts[2], "%d", &major)
		fmt.Sscanf(parts[3], "%d", &minor)
	} else if len(parts) >= 2 {
		fmt.Sscanf(parts[0], "%d", &major)
		fmt.Sscanf(parts[1], "%d", &minor)
	}
	return
}
