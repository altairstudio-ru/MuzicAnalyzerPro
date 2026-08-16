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
    lyrics_path   TEXT NOT NULL DEFAULT '',
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

CREATE TABLE IF NOT EXISTS analysis_results (
    track_id    TEXT PRIMARY KEY REFERENCES tracks(id) ON DELETE CASCADE,
    version     INTEGER NOT NULL DEFAULT 1,
    status      TEXT NOT NULL DEFAULT 'pending',
    error_msg   TEXT NOT NULL DEFAULT '',
    result_json TEXT NOT NULL DEFAULT '{}',
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS albums (
    id         TEXT PRIMARY KEY,
    title      TEXT NOT NULL DEFAULT '',
    kind       TEXT NOT NULL DEFAULT 'compilation',
    notes      TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS album_tracks (
    album_id  TEXT NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
    track_id  TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    position  INTEGER NOT NULL DEFAULT 0,
    notes     TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (album_id, track_id)
);
CREATE INDEX IF NOT EXISTS idx_album_tracks_album_pos ON album_tracks(album_id, position);
CREATE INDEX IF NOT EXISTS idx_album_tracks_track ON album_tracks(track_id);

CREATE TABLE IF NOT EXISTS labels (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    color      TEXT NOT NULL DEFAULT '#ffb454',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS track_labels (
    track_id TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    label_id TEXT NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    PRIMARY KEY (track_id, label_id)
);
CREATE INDEX IF NOT EXISTS idx_track_labels_label ON track_labels(label_id);

CREATE TABLE IF NOT EXISTS variant_groups (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL DEFAULT '',
    notes         TEXT NOT NULL DEFAULT '',
    best_track_id TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS variant_group_tracks (
    group_id TEXT NOT NULL REFERENCES variant_groups(id) ON DELETE CASCADE,
    track_id TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, track_id)
);
CREATE INDEX IF NOT EXISTS idx_variant_group_tracks_track ON variant_group_tracks(track_id);
`
