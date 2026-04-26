package store

import (
	"context"
	"testing"
	"time"

	"github.com/deploymenttheory/go-sdk-windowsuup/catalog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func sampleBuild() catalog.Build {
	return catalog.Build{
		UUID:         "test-uuid-0001",
		Revision:     200,
		Title:        "Windows 11 Insider Preview Feature Update (26120.4061)",
		Build:        "10.0.26120.4061",
		MajorVersion: 26120,
		MinorVersion: 4061,
		Arch:         "amd64",
		Ring:         "Dev",
		Flight:       "Active",
		Branch:       "rs_prerelease",
		SKU:          48,
		IsStable:     false,
		IsInsider:    true,
		IsCumulative: false,
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
		DiscoveredAt: time.Now().UTC().Truncate(time.Second),
	}
}

func TestOpen(t *testing.T) {
	s := openTestDB(t)
	assert.NoError(t, s.Ping(context.Background()))
}

func TestUpsertAndGetBuild(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	b := sampleBuild()

	isNew, err := s.UpsertBuild(ctx, b)
	require.NoError(t, err)
	assert.True(t, isNew, "first insert must be new")

	got, err := s.GetBuild(ctx, b.UUID)
	require.NoError(t, err)
	assert.Equal(t, b.UUID, got.UUID)
	assert.Equal(t, b.Title, got.Title)
	assert.Equal(t, b.Ring, got.Ring)
	assert.True(t, got.IsInsider)
	assert.False(t, got.IsStable)

	// Second upsert must not be new.
	isNew, err = s.UpsertBuild(ctx, b)
	require.NoError(t, err)
	assert.False(t, isNew, "second upsert must not be new")
}

func TestGetBuildNotFound(t *testing.T) {
	s := openTestDB(t)
	_, err := s.GetBuild(context.Background(), "does-not-exist")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestListBuilds(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	builds := []catalog.Build{
		{UUID: "u1", Title: "Windows 11 Feature Update (26120.1)", Build: "10.0.26120.1", MajorVersion: 26120, MinorVersion: 1, Arch: "amd64", Ring: "Dev"},
		{UUID: "u2", Title: "Windows 11 Feature Update (26120.2)", Build: "10.0.26120.2", MajorVersion: 26120, MinorVersion: 2, Arch: "arm64", Ring: "Dev"},
		{UUID: "u3", Title: "Windows 11 Feature Update (26100.1)", Build: "10.0.26100.1", MajorVersion: 26100, MinorVersion: 1, Arch: "amd64", Ring: "Retail", IsStable: true},
	}
	for _, b := range builds {
		_, err := s.UpsertBuild(ctx, b)
		require.NoError(t, err)
	}

	// All builds.
	all, total, err := s.ListBuilds(ctx, catalog.BuildQuery{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, all, 3)

	// Filter by arch.
	amd64, total, err := s.ListBuilds(ctx, catalog.BuildQuery{Arch: "amd64", Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, amd64, 2)

	// Filter stable only.
	stable, total, err := s.ListBuilds(ctx, catalog.BuildQuery{StableOnly: true, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, stable, 1)
	assert.Equal(t, "u3", stable[0].UUID)

	// Search.
	searched, total, err := s.ListBuilds(ctx, catalog.BuildQuery{Search: "26100", Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	_ = searched
}

func TestDeleteBuild(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	b := sampleBuild()

	_, err := s.UpsertBuild(ctx, b)
	require.NoError(t, err)

	require.NoError(t, s.DeleteBuild(ctx, b.UUID))

	_, err = s.GetBuild(ctx, b.UUID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestUpsertAndGetFiles(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	b := sampleBuild()
	_, err := s.UpsertBuild(ctx, b)
	require.NoError(t, err)

	files := []catalog.File{
		{UUID: b.UUID, Name: "Windows11.esd", SHA1: "abc", SHA256: "def", SizeBytes: 1024, FileType: catalog.FileTypeESD},
		{UUID: b.UUID, Name: "lang_en-us.cab", SHA1: "ghi", SHA256: "jkl", SizeBytes: 512, FileType: catalog.FileTypeCAB},
	}
	require.NoError(t, s.UpsertFiles(ctx, b.UUID, files))

	got, err := s.GetFiles(ctx, b.UUID)
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestFeedOperations(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	entry := catalog.FeedEntry{
		EventType:   string(catalog.EventBuildDiscovered),
		BuildUUID:   "test-uuid",
		BuildTitle:  "Windows 11 26120",
		BuildNumber: "26120.1",
		Arch:        "amd64",
		Ring:        "Dev",
		OccurredAt:  time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, s.AppendFeedEntry(ctx, entry))

	entries, total, err := s.GetFeed(ctx, catalog.FeedQuery{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, entries, 1)
	assert.Equal(t, entry.BuildUUID, entries[0].BuildUUID)
}

func TestGetFilesForDiff(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	b1 := catalog.Build{UUID: "build-a", Title: "Build A", Build: "1.0", MajorVersion: 1, MinorVersion: 0, Arch: "amd64", Ring: "Dev"}
	b2 := catalog.Build{UUID: "build-b", Title: "Build B", Build: "1.1", MajorVersion: 1, MinorVersion: 1, Arch: "amd64", Ring: "Dev"}
	for _, b := range []catalog.Build{b1, b2} {
		_, err := s.UpsertBuild(ctx, b)
		require.NoError(t, err)
	}
	require.NoError(t, s.UpsertFiles(ctx, "build-a", []catalog.File{{UUID: "build-a", Name: "file.esd", SHA1: "aaa", FileType: catalog.FileTypeESD}}))
	require.NoError(t, s.UpsertFiles(ctx, "build-b", []catalog.File{{UUID: "build-b", Name: "file.esd", SHA1: "bbb", FileType: catalog.FileTypeESD}}))

	filesA, filesB, err := s.GetFilesForDiff(ctx, "build-a", "build-b")
	require.NoError(t, err)
	assert.Len(t, filesA, 1)
	assert.Len(t, filesB, 1)
	assert.Equal(t, "aaa", filesA[0].SHA1)
	assert.Equal(t, "bbb", filesB[0].SHA1)
}
