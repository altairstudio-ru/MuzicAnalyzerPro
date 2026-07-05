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

	return db, nil
}
