package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

// UpsertTrack inserts or replaces a track in the database.
func UpsertTrack(db *sql.DB, t *models.Track) error {
	tagsJSON, err := json.Marshal(t.Tags)
	if err != nil {
		return fmt.Errorf("marshal tags: %w", err)
	}

	dl := 0
	if t.IsDownloaded {
		dl = 1
	}

	_, err = db.Exec(`
		INSERT INTO tracks (id, title, artist, prompt, lyrics, tags, workspace,
		                    duration, created_at, audio_path, audio_hash,
		                    is_downloaded, file_size)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title         = excluded.title,
			artist        = excluded.artist,
			prompt        = excluded.prompt,
			lyrics        = excluded.lyrics,
			tags          = excluded.tags,
			workspace     = excluded.workspace,
			duration      = excluded.duration,
			created_at    = excluded.created_at,
			audio_path    = excluded.audio_path,
			audio_hash    = excluded.audio_hash,
			is_downloaded = excluded.is_downloaded,
			file_size     = excluded.file_size,
			updated_at    = datetime('now')`,
		t.ID, t.Title, t.Artist, t.Prompt, t.Lyrics, string(tagsJSON),
		t.Workspace, t.Duration, t.CreatedAt, t.AudioPath, t.AudioHash,
		dl, t.FileSize,
	)
	if err != nil {
		return fmt.Errorf("upsert track: %w", err)
	}
	return nil
}

// GetTrack retrieves a single track by its ID.
func GetTrack(db *sql.DB, id string) (*models.Track, error) {
	row := db.QueryRow(`
		SELECT id, title, artist, prompt, lyrics, tags, workspace,
		       duration, created_at, audio_path, audio_hash,
		       is_downloaded, file_size
		FROM tracks WHERE id = ?`, id)

	t := &models.Track{}
	var tagsJSON string
	var dl int
	err := row.Scan(&t.ID, &t.Title, &t.Artist, &t.Prompt, &t.Lyrics,
		&tagsJSON, &t.Workspace, &t.Duration, &t.CreatedAt,
		&t.AudioPath, &t.AudioHash, &dl, &t.FileSize)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get track: %w", err)
	}
	t.IsDownloaded = dl == 1

	if err := json.Unmarshal([]byte(tagsJSON), &t.Tags); err != nil {
		t.Tags = []string{}
	}

	return t, nil
}

// ListTracks returns tracks matching the given filter criteria.
func ListTracks(db *sql.DB, filter models.TrackFilter) ([]models.Track, error) {
	var conditions []string
	var args []interface{}

	if filter.Workspace != "" {
		conditions = append(conditions, "workspace = ?")
		args = append(args, filter.Workspace)
	}
	if filter.Tag != "" {
		conditions = append(conditions, "tags LIKE ?")
		args = append(args, "%"+filter.Tag+"%")
	}
	if filter.Search != "" {
		conditions = append(conditions,
			"(title LIKE ? OR prompt LIKE ? OR lyrics LIKE ?)")
		s := "%" + filter.Search + "%"
		args = append(args, s, s, s)
	}
	if filter.Downloaded != nil {
		v := 0
		if *filter.Downloaded {
			v = 1
		}
		conditions = append(conditions, "is_downloaded = ?")
		args = append(args, v)
	}

	query := "SELECT id, title, artist, prompt, lyrics, tags, workspace, " +
		"duration, created_at, audio_path, audio_hash, " +
		"is_downloaded, file_size FROM tracks"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	} else {
		query += " LIMIT 50"
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tracks: %w", err)
	}
	defer rows.Close()

	var tracks []models.Track
	for rows.Next() {
		t := models.Track{}
		var tagsJSON string
		var dl int
		if err := rows.Scan(&t.ID, &t.Title, &t.Artist, &t.Prompt,
			&t.Lyrics, &tagsJSON, &t.Workspace, &t.Duration,
			&t.CreatedAt, &t.AudioPath, &t.AudioHash,
			&dl, &t.FileSize); err != nil {
			return nil, fmt.Errorf("scan track: %w", err)
		}
		t.IsDownloaded = dl == 1
		if err := json.Unmarshal([]byte(tagsJSON), &t.Tags); err != nil {
			t.Tags = []string{}
		}
		tracks = append(tracks, t)
	}
	if tracks == nil {
		tracks = []models.Track{}
	}
	return tracks, rows.Err()
}

// DeleteTrack removes a track by its ID.
func DeleteTrack(db *sql.DB, id string) error {
	_, err := db.Exec("DELETE FROM tracks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete track: %w", err)
	}
	return nil
}

// GetTrackCount returns the total number of tracks in the database.
func GetTrackCount(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM tracks").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("get track count: %w", err)
	}
	return count, nil
}

// GetDownloadedCount returns the number of downloaded tracks.
func GetDownloadedCount(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM tracks WHERE is_downloaded = 1").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("get downloaded count: %w", err)
	}
	return count, nil
}
