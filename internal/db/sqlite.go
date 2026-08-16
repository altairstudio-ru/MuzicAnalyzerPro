package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Init opens (or creates) the SQLite database at the given path,
// enables WAL mode and foreign keys, and runs migrations.
func Init(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Enable WAL mode for better concurrent performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("enable WAL: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	// Run migrations
	if _, err := db.Exec(schemaSQL); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	// Migrate existing databases: add columns if missing (errors ignored when already present)
	db.Exec("ALTER TABLE tracks ADD COLUMN lyrics_path TEXT NOT NULL DEFAULT ''")
	db.Exec("ALTER TABLE tracks ADD COLUMN upvote_count INTEGER NOT NULL DEFAULT 0")
	db.Exec("ALTER TABLE tracks ADD COLUMN play_count INTEGER NOT NULL DEFAULT 0")
	db.Exec("ALTER TABLE tracks ADD COLUMN is_liked INTEGER NOT NULL DEFAULT 0")
	db.Exec("ALTER TABLE tracks ADD COLUMN track_type TEXT NOT NULL DEFAULT ''")
	db.Exec("ALTER TABLE tracks ADD COLUMN model_name TEXT NOT NULL DEFAULT ''")

	// Seed default labels for curation (single, album, compilation, ...)
	if err := EnsureDefaultLabels(db); err != nil {
		return nil, fmt.Errorf("seed default labels: %w", err)
	}

	return db, nil
}
