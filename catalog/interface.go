package catalog

import "context"

// Store is the persistence interface for the build catalog.
// catalog/store/sqlite.go provides the production SQLite implementation.
// All methods must be safe for concurrent use.
type Store interface {
	// Build operations.
	UpsertBuild(ctx context.Context, b Build) (isNew bool, err error)
	GetBuild(ctx context.Context, uuid string) (*Build, error)
	ListBuilds(ctx context.Context, q BuildQuery) (builds []Build, total int64, err error)
	DeleteBuild(ctx context.Context, uuid string) error

	// File operations — files are always associated with a build UUID.
	UpsertFiles(ctx context.Context, uuid string, files []File) error
	GetFiles(ctx context.Context, uuid string) ([]File, error)

	// Feed / change history.
	AppendFeedEntry(ctx context.Context, e FeedEntry) error
	GetFeed(ctx context.Context, q FeedQuery) (entries []FeedEntry, total int64, err error)

	// Diff support: retrieve file sets for two builds in one round-trip.
	GetFilesForDiff(ctx context.Context, uuidA, uuidB string) (a []File, b []File, err error)

	// Lifecycle.
	Ping(ctx context.Context) error
	Close() error
}

// EventEmitter broadcasts build lifecycle events to registered handlers.
// All methods must be safe for concurrent use.
type EventEmitter interface {
	// Emit broadcasts e to all currently registered handlers.
	Emit(ctx context.Context, e BuildEvent)
	// Subscribe registers h and returns a function that removes it.
	Subscribe(h EventHandler) (unsubscribe func())
}

// EventHandler receives build lifecycle events.
type EventHandler interface {
	HandleEvent(ctx context.Context, e BuildEvent)
}

// EventHandlerFunc is a function adapter that implements EventHandler.
type EventHandlerFunc func(ctx context.Context, e BuildEvent)

func (f EventHandlerFunc) HandleEvent(ctx context.Context, e BuildEvent) { f(ctx, e) }

// BuildEvent is the payload emitted for every build lifecycle change.
type BuildEvent struct {
	// Type classifies the lifecycle transition.
	Type EventType
	// Build is the current state of the build after the transition.
	Build Build
	// PreviousBuild is non-nil only for EventBuildUpdated.
	PreviousBuild *Build
}

// EventType classifies a build lifecycle event.
type EventType string

const (
	// EventBuildDiscovered is emitted the first time a build UUID is seen.
	EventBuildDiscovered EventType = "build.discovered"
	// EventBuildUpdated is emitted when an existing build's metadata changes.
	EventBuildUpdated EventType = "build.updated"
	// EventBuildDeleted is emitted when a build is removed from the catalog.
	EventBuildDeleted EventType = "build.deleted"
)
