-- 002_indexes.sql: performance indexes on builds and files.

CREATE INDEX IF NOT EXISTS idx_builds_arch           ON builds(arch);
CREATE INDEX IF NOT EXISTS idx_builds_ring           ON builds(ring);
CREATE INDEX IF NOT EXISTS idx_builds_is_stable      ON builds(is_stable);
CREATE INDEX IF NOT EXISTS idx_builds_major_version  ON builds(major_version DESC);
CREATE INDEX IF NOT EXISTS idx_builds_discovered_at  ON builds(discovered_at DESC);

CREATE INDEX IF NOT EXISTS idx_files_uuid      ON files(uuid);
CREATE INDEX IF NOT EXISTS idx_files_file_type ON files(file_type);
