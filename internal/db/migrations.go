package db

// schemaSQL contains all DDL statements for the SQLite database.
const schemaSQL = `
CREATE TABLE IF NOT EXISTS tracks (
    id            TEXT PRIMARY KEY,
    title         TEXT NOT NULL DEFAULT '',
    artist        TEXT NOT NULL DEFAULT '',
    prompt        TEXT NOT NULL DEFAULT '',
    lyrics        TEXT NOT NULL DEFAULT '',
    tags          TEXT NOT NULL DEFAULT '[]',
    workspace     TEXT NOT NULL DEFAULT '',
    duration      INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL DEFAULT '',
    audio_path    TEXT NOT NULL DEFAULT '',
    audio_hash    TEXT NOT NULL DEFAULT '',
    is_downloaded INTEGER NOT NULL DEFAULT 0,
    file_size     INTEGER NOT NULL DEFAULT 0,
    updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS workspaces (
    name        TEXT PRIMARY KEY,
    track_count INTEGER NOT NULL DEFAULT 0,
    synced_at   TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_tracks_workspace ON tracks(workspace);
CREATE INDEX IF NOT EXISTS idx_tracks_created_at ON tracks(created_at);
`
