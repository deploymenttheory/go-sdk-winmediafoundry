-- 003_feed.sql: build change-feed / history table.

CREATE TABLE IF NOT EXISTS feed (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type   TEXT    NOT NULL,
    build_uuid   TEXT    NOT NULL,
    build_title  TEXT    NOT NULL DEFAULT '',
    build_number TEXT    NOT NULL DEFAULT '',
    arch         TEXT    NOT NULL DEFAULT '',
    ring         TEXT    NOT NULL DEFAULT '',
    occurred_at  DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    payload      BLOB
);

CREATE INDEX IF NOT EXISTS idx_feed_occurred_at ON feed(occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_feed_event_type  ON feed(event_type);
CREATE INDEX IF NOT EXISTS idx_feed_build_uuid  ON feed(build_uuid);
