package db

import (
	"database/sql"
	"fmt"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

// UpsertWorkspace inserts or replaces a workspace.
func UpsertWorkspace(db *sql.DB, w *models.Workspace) error {
	_, err := db.Exec(`
		INSERT INTO workspaces (name, track_count, synced_at)
		VALUES (?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			track_count = excluded.track_count,
			synced_at    = excluded.synced_at`,
		w.Name, w.TrackCount, w.SyncedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert workspace: %w", err)
	}
	return nil
}

// ListWorkspaces returns all workspaces ordered by name.
func ListWorkspaces(db *sql.DB) ([]models.Workspace, error) {
	rows, err := db.Query(`
		SELECT name, track_count, synced_at
		FROM workspaces ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer rows.Close()

	var workspaces []models.Workspace
	for rows.Next() {
		var w models.Workspace
		if err := rows.Scan(&w.Name, &w.TrackCount, &w.SyncedAt); err != nil {
			return nil, fmt.Errorf("scan workspace: %w", err)
		}
		workspaces = append(workspaces, w)
	}
	if workspaces == nil {
		workspaces = []models.Workspace{}
	}
	return workspaces, rows.Err()
}

// DeleteWorkspace removes a workspace by name.
func DeleteWorkspace(db *sql.DB, name string) error {
	_, err := db.Exec("DELETE FROM workspaces WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("delete workspace: %w", err)
	}
	return nil
}

// UpdateWorkspaceTrackCount recalculates and updates the track count for a workspace.
func UpdateWorkspaceTrackCount(db *sql.DB, name string) error {
	_, err := db.Exec(`
		UPDATE workspaces
		SET track_count = (SELECT COUNT(*) FROM tracks WHERE workspace = ?),
		    synced_at = datetime('now')
		WHERE name = ?`, name, name)
	if err != nil {
		return fmt.Errorf("update workspace track count: %w", err)
	}
	return nil
}
