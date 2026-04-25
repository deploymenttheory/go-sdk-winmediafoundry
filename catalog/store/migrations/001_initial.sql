-- 001_initial.sql: builds and files tables.

CREATE TABLE IF NOT EXISTS builds (
    uuid            TEXT    PRIMARY KEY,
    revision        INTEGER NOT NULL DEFAULT 1,
    title           TEXT    NOT NULL,
    build           TEXT    NOT NULL,
    major_version   INTEGER NOT NULL DEFAULT 0,
    minor_version   INTEGER NOT NULL DEFAULT 0,
    arch            TEXT    NOT NULL DEFAULT '',
    ring            TEXT    NOT NULL DEFAULT '',
    flight          TEXT    NOT NULL DEFAULT '',
    branch          TEXT    NOT NULL DEFAULT '',
    sku             INTEGER NOT NULL DEFAULT 0,
    is_stable       INTEGER NOT NULL DEFAULT 0,
    is_insider      INTEGER NOT NULL DEFAULT 0,
    is_cumulative   INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    discovered_at   DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    updated_at      DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE IF NOT EXISTS files (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid        TEXT    NOT NULL REFERENCES builds(uuid) ON DELETE CASCADE,
    name        TEXT    NOT NULL,
    lang        TEXT    NOT NULL DEFAULT '',
    edition     TEXT    NOT NULL DEFAULT '',
    sha1        TEXT    NOT NULL DEFAULT '',
    sha256      TEXT    NOT NULL DEFAULT '',
    size_bytes  INTEGER NOT NULL DEFAULT 0,
    file_type   TEXT    NOT NULL DEFAULT 'unknown',
    modified_at DATETIME,
    UNIQUE(uuid, name, lang, edition)
);
