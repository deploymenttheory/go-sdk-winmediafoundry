// Package store provides a SQLite-backed implementation of catalog.Store.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/deploymenttheory/go-sdk-windowsuup/catalog"
	_ "modernc.org/sqlite" // SQLite driver
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// SQLiteStore is a catalog.Store backed by a SQLite database.
// It is safe for concurrent use (WAL journal mode is enabled).
type SQLiteStore struct {
	db *sql.DB
}

// Open opens (or creates) a SQLite database at the given path and runs
// all pending schema migrations.
//
// Use ":memory:" for an ephemeral in-memory database (useful in tests).
func Open(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Enable WAL mode for concurrent reads alongside writes.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	// Enforce foreign key constraints.
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Ping implements catalog.Store.
func (s *SQLiteStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close implements catalog.Store.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// UpsertBuild implements catalog.Store.
// Returns isNew=true when the UUID did not previously exist in the database.
func (s *SQLiteStore) UpsertBuild(ctx context.Context, b catalog.Build) (bool, error) {
	now := time.Now().UTC()

	// Check if already exists.
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM builds WHERE uuid = ?`, b.UUID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check build existence: %w", err)
	}
	isNew := count == 0

	const q = `
INSERT INTO builds
	(uuid, revision, title, build, major_version, minor_version,
	 arch, ring, flight, branch, sku,
	 is_stable, is_insider, is_cumulative,
	 created_at, discovered_at, updated_at)
VALUES
	(?, ?, ?, ?, ?, ?,
	 ?, ?, ?, ?, ?,
	 ?, ?, ?,
	 ?, ?, ?)
ON CONFLICT(uuid) DO UPDATE SET
	revision      = excluded.revision,
	title         = excluded.title,
	build         = excluded.build,
	major_version = excluded.major_version,
	minor_version = excluded.minor_version,
	arch          = excluded.arch,
	ring          = excluded.ring,
	flight        = excluded.flight,
	branch        = excluded.branch,
	sku           = excluded.sku,
	is_stable     = excluded.is_stable,
	is_insider    = excluded.is_insider,
	is_cumulative = excluded.is_cumulative,
	updated_at    = excluded.updated_at`

	createdAt := b.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	discoveredAt := b.DiscoveredAt
	if discoveredAt.IsZero() {
		discoveredAt = now
	}

	_, err = s.db.ExecContext(ctx, q,
		b.UUID, b.Revision, b.Title, b.Build, b.MajorVersion, b.MinorVersion,
		b.Arch, b.Ring, b.Flight, b.Branch, b.SKU,
		boolToInt(b.IsStable), boolToInt(b.IsInsider), boolToInt(b.IsCumulative),
		createdAt.UTC().Format(time.RFC3339),
		discoveredAt.UTC().Format(time.RFC3339),
		now.Format(time.RFC3339),
	)
	if err != nil {
		return false, fmt.Errorf("upsert build %s: %w", b.UUID, err)
	}
	return isNew, nil
}

// GetBuild implements catalog.Store.
func (s *SQLiteStore) GetBuild(ctx context.Context, uuid string) (*catalog.Build, error) {
	const q = `
SELECT uuid, revision, title, build, major_version, minor_version,
       arch, ring, flight, branch, sku,
       is_stable, is_insider, is_cumulative,
       created_at, discovered_at, updated_at
FROM builds WHERE uuid = ?`

	row := s.db.QueryRowContext(ctx, q, uuid)
	b, err := scanBuild(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return b, err
}

// ListBuilds implements catalog.Store.
func (s *SQLiteStore) ListBuilds(ctx context.Context, q catalog.BuildQuery) ([]catalog.Build, int64, error) {
	where, args := buildWhereClause(q)
	orderBy := safeOrderBy(q.OrderBy, q.Desc)

	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	countSQL := "SELECT COUNT(*) FROM builds" + where
	var total int64
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count builds: %w", err)
	}

	listSQL := `SELECT uuid, revision, title, build, major_version, minor_version,
       arch, ring, flight, branch, sku,
       is_stable, is_insider, is_cumulative,
       created_at, discovered_at, updated_at
FROM builds` + where + orderBy + fmt.Sprintf(" LIMIT %d OFFSET %d", limit, q.Offset)

	rows, err := s.db.QueryContext(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list builds: %w", err)
	}
	defer rows.Close()

	var builds []catalog.Build
	for rows.Next() {
		b, err := scanBuild(rows)
		if err != nil {
			return nil, 0, err
		}
		builds = append(builds, *b)
	}
	return builds, total, rows.Err()
}

// DeleteBuild implements catalog.Store.
func (s *SQLiteStore) DeleteBuild(ctx context.Context, uuid string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM builds WHERE uuid = ?`, uuid)
	return err
}

// UpsertFiles implements catalog.Store.
func (s *SQLiteStore) UpsertFiles(ctx context.Context, uuid string, files []catalog.File) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	const q = `
INSERT INTO files (uuid, name, lang, edition, sha1, sha256, size_bytes, file_type, modified_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(uuid, name, lang, edition) DO UPDATE SET
	sha1       = excluded.sha1,
	sha256     = excluded.sha256,
	size_bytes = excluded.size_bytes,
	file_type  = excluded.file_type,
	modified_at = excluded.modified_at`

	for _, f := range files {
		var modifiedAt *string
		if !f.ModifiedAt.IsZero() {
			s := f.ModifiedAt.UTC().Format(time.RFC3339)
			modifiedAt = &s
		}
		if _, err := tx.ExecContext(ctx, q,
			uuid, f.Name, f.Lang, f.Edition,
			f.SHA1, f.SHA256, f.SizeBytes, string(f.FileType),
			modifiedAt,
		); err != nil {
			return fmt.Errorf("upsert file %s: %w", f.Name, err)
		}
	}
	return tx.Commit()
}

// GetFiles implements catalog.Store.
func (s *SQLiteStore) GetFiles(ctx context.Context, uuid string) ([]catalog.File, error) {
	const q = `
SELECT uuid, name, lang, edition, sha1, sha256, size_bytes, file_type, modified_at
FROM files WHERE uuid = ? ORDER BY name`

	rows, err := s.db.QueryContext(ctx, q, uuid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []catalog.File
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, *f)
	}
	return files, rows.Err()
}

// AppendFeedEntry implements catalog.Store.
func (s *SQLiteStore) AppendFeedEntry(ctx context.Context, e catalog.FeedEntry) error {
	const q = `
INSERT INTO feed (event_type, build_uuid, build_title, build_number, arch, ring, occurred_at, payload)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	occurredAt := e.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, q,
		e.EventType, e.BuildUUID, e.BuildTitle, e.BuildNumber,
		e.Arch, e.Ring, occurredAt.UTC().Format(time.RFC3339), e.Payload,
	)
	return err
}

// GetFeed implements catalog.Store.
func (s *SQLiteStore) GetFeed(ctx context.Context, q catalog.FeedQuery) ([]catalog.FeedEntry, int64, error) {
	var conditions []string
	var args []interface{}

	if !q.Since.IsZero() {
		conditions = append(conditions, "occurred_at >= ?")
		args = append(args, q.Since.UTC().Format(time.RFC3339))
	}
	if q.EventType != "" {
		conditions = append(conditions, "event_type = ?")
		args = append(args, q.EventType)
	}

	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM feed"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}

	listSQL := `SELECT id, event_type, build_uuid, build_title, build_number, arch, ring, occurred_at, payload
FROM feed` + where + fmt.Sprintf(" ORDER BY occurred_at DESC LIMIT %d OFFSET %d", limit, q.Offset)

	rows, err := s.db.QueryContext(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []catalog.FeedEntry
	for rows.Next() {
		e, err := scanFeedEntry(rows)
		if err != nil {
			return nil, 0, err
		}
		entries = append(entries, *e)
	}
	return entries, total, rows.Err()
}

// GetFilesForDiff implements catalog.Store.
func (s *SQLiteStore) GetFilesForDiff(ctx context.Context, uuidA, uuidB string) ([]catalog.File, []catalog.File, error) {
	filesA, err := s.GetFiles(ctx, uuidA)
	if err != nil {
		return nil, nil, fmt.Errorf("files for %s: %w", uuidA, err)
	}
	filesB, err := s.GetFiles(ctx, uuidB)
	if err != nil {
		return nil, nil, fmt.Errorf("files for %s: %w", uuidB, err)
	}
	return filesA, filesB, nil
}

// ─── Scanning helpers ──────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanBuild(row scanner) (*catalog.Build, error) {
	var b catalog.Build
	var createdAt, discoveredAt, updatedAt string
	var isStable, isInsider, isCumulative int

	err := row.Scan(
		&b.UUID, &b.Revision, &b.Title, &b.Build, &b.MajorVersion, &b.MinorVersion,
		&b.Arch, &b.Ring, &b.Flight, &b.Branch, &b.SKU,
		&isStable, &isInsider, &isCumulative,
		&createdAt, &discoveredAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	b.IsStable = isStable == 1
	b.IsInsider = isInsider == 1
	b.IsCumulative = isCumulative == 1
	b.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	b.DiscoveredAt, _ = time.Parse(time.RFC3339, discoveredAt)
	b.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &b, nil
}

func scanFile(row scanner) (*catalog.File, error) {
	var f catalog.File
	var modifiedAt *string
	var fileType string

	err := row.Scan(
		&f.UUID, &f.Name, &f.Lang, &f.Edition,
		&f.SHA1, &f.SHA256, &f.SizeBytes, &fileType, &modifiedAt,
	)
	if err != nil {
		return nil, err
	}
	f.FileType = catalog.FileType(fileType)
	if modifiedAt != nil {
		f.ModifiedAt, _ = time.Parse(time.RFC3339, *modifiedAt)
	}
	return &f, nil
}

func scanFeedEntry(row scanner) (*catalog.FeedEntry, error) {
	var e catalog.FeedEntry
	var occurredAt string

	err := row.Scan(
		&e.ID, &e.EventType, &e.BuildUUID, &e.BuildTitle, &e.BuildNumber,
		&e.Arch, &e.Ring, &occurredAt, &e.Payload,
	)
	if err != nil {
		return nil, err
	}
	e.OccurredAt, _ = time.Parse(time.RFC3339, occurredAt)
	return &e, nil
}

// ─── Query builder helpers ─────────────────────────────────────────────────

func buildWhereClause(q catalog.BuildQuery) (string, []interface{}) {
	var conditions []string
	var args []interface{}

	if q.Search != "" {
		conditions = append(conditions, "title LIKE ?")
		args = append(args, "%"+q.Search+"%")
	}
	if q.Arch != "" {
		conditions = append(conditions, "arch = ?")
		args = append(args, q.Arch)
	}
	if q.Ring != "" {
		conditions = append(conditions, "ring = ?")
		args = append(args, q.Ring)
	}
	if q.StableOnly {
		conditions = append(conditions, "is_stable = 1")
	}

	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

func safeOrderBy(field string, desc bool) string {
	allowed := map[string]string{
		"created_at":    "created_at",
		"discovered_at": "discovered_at",
		"build_number":  "major_version, minor_version",
		"build":         "major_version, minor_version",
		"":              "discovered_at",
	}
	col, ok := allowed[field]
	if !ok {
		col = "discovered_at"
	}
	dir := "DESC"
	if !desc {
		dir = "DESC" // default to newest-first
	}
	return " ORDER BY " + col + " " + dir
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
