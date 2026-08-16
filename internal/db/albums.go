package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

// CreateAlbum inserts a new album/collection.
func CreateAlbum(db *sql.DB, a *models.Album) error {
	if a.ID == "" {
		a.ID = newID("alb")
	}
	if a.Kind == "" {
		a.Kind = "compilation"
	}
	if a.Title == "" {
		return fmt.Errorf("album title is required")
	}
	_, err := db.Exec(`
		INSERT INTO albums (id, title, kind, notes)
		VALUES (?, ?, ?, ?)`,
		a.ID, a.Title, a.Kind, a.Notes,
	)
	if err != nil {
		return fmt.Errorf("create album: %w", err)
	}
	return nil
}

// ListAlbums returns all albums with track counts, newest first.
func ListAlbums(db *sql.DB) ([]models.Album, error) {
	rows, err := db.Query(`
		SELECT a.id, a.title, a.kind, a.notes, a.created_at, a.updated_at,
		       (SELECT COUNT(*) FROM album_tracks at WHERE at.album_id = a.id)
		FROM albums a
		ORDER BY a.updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list albums: %w", err)
	}
	defer rows.Close()

	albums := []models.Album{}
	for rows.Next() {
		a := models.Album{}
		if err := rows.Scan(&a.ID, &a.Title, &a.Kind, &a.Notes,
			&a.CreatedAt, &a.UpdatedAt, &a.TrackCount); err != nil {
			return nil, fmt.Errorf("scan album: %w", err)
		}
		albums = append(albums, a)
	}
	return albums, rows.Err()
}

// GetAlbumWithTracks returns an album with its ordered tracklist.
// Returns nil, nil if the album does not exist.
func GetAlbumWithTracks(db *sql.DB, id string) (*models.AlbumWithTracks, error) {
	row := db.QueryRow(`
		SELECT id, title, kind, notes, created_at, updated_at
		FROM albums WHERE id = ?`, id)

	var out models.AlbumWithTracks
	err := row.Scan(&out.Album.ID, &out.Album.Title, &out.Album.Kind,
		&out.Album.Notes, &out.Album.CreatedAt, &out.Album.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get album: %w", err)
	}

	rows, err := db.Query(`
		SELECT t.id, t.title, t.artist, t.prompt, t.lyrics, t.tags, t.workspace,
		       t.duration, t.created_at, t.audio_path, t.audio_hash,
		       t.lyrics_path, t.is_downloaded, t.file_size,
		       at.position, at.notes
		FROM album_tracks at
		JOIN tracks t ON t.id = at.track_id
		WHERE at.album_id = ?
		ORDER BY at.position ASC`, id)
	if err != nil {
		return nil, fmt.Errorf("get album tracks: %w", err)
	}
	defer rows.Close()

	out.Tracks = []models.AlbumTrackItem{}
	for rows.Next() {
		item := models.AlbumTrackItem{}
		tr := &item.Track
		var tagsJSON string
		var dl int
		if err := rows.Scan(&tr.ID, &tr.Title, &tr.Artist, &tr.Prompt,
			&tr.Lyrics, &tagsJSON, &tr.Workspace, &tr.Duration, &tr.CreatedAt,
			&tr.AudioPath, &tr.AudioHash, &tr.LyricsPath, &dl, &tr.FileSize,
			&item.Position, &item.Notes); err != nil {
			return nil, fmt.Errorf("scan album track: %w", err)
		}
		tr.IsDownloaded = dl == 1
		if err := unmarshalTags(tagsJSON, &tr.Tags); err != nil {
			return nil, err
		}
		out.TotalDuration += tr.Duration
		out.Tracks = append(out.Tracks, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("album tracks rows: %w", err)
	}

	out.Album.TrackCount = len(out.Tracks)
	return &out, nil
}

// UpdateAlbum updates title, kind and notes of an album.
func UpdateAlbum(db *sql.DB, id string, title, kind, notes string) error {
	res, err := db.Exec(`
		UPDATE albums SET title = ?, kind = ?, notes = ?, updated_at = datetime('now')
		WHERE id = ?`,
		title, kind, notes, id,
	)
	if err != nil {
		return fmt.Errorf("update album: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteAlbum removes an album (and its tracklist via CASCADE).
func DeleteAlbum(db *sql.DB, id string) error {
	res, err := db.Exec("DELETE FROM albums WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete album: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// AddTrackToAlbum adds a track to an album tracklist. When position is 0 the
// track is appended at the end. Duplicate entries are ignored.
func AddTrackToAlbum(db *sql.DB, albumID, trackID string, position int, notes string) error {
	if position <= 0 {
		var max int
		_ = db.QueryRow("SELECT COALESCE(MAX(position), 0) FROM album_tracks WHERE album_id = ?", albumID).Scan(&max)
		position = max + 1
	}
	_, err := db.Exec(`
		INSERT INTO album_tracks (album_id, track_id, position, notes)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(album_id, track_id) DO UPDATE SET position = excluded.position, notes = excluded.notes`,
		albumID, trackID, position, notes,
	)
	if err != nil {
		if isForeignKeyError(err) {
			return fmt.Errorf("add track to album: %w (album or track does not exist)", err)
		}
		return fmt.Errorf("add track to album: %w", err)
	}
	_, _ = db.Exec("UPDATE albums SET updated_at = datetime('now') WHERE id = ?", albumID)
	return nil
}

// RemoveTrackFromAlbum removes a track from an album tracklist.
func RemoveTrackFromAlbum(db *sql.DB, albumID, trackID string) error {
	res, err := db.Exec("DELETE FROM album_tracks WHERE album_id = ? AND track_id = ?", albumID, trackID)
	if err != nil {
		return fmt.Errorf("remove track from album: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	_, _ = db.Exec("UPDATE albums SET updated_at = datetime('now') WHERE id = ?", albumID)
	return nil
}

// SetAlbumTrackPosition updates the position of a single track in the album.
func SetAlbumTrackPosition(db *sql.DB, albumID, trackID string, position int) error {
	res, err := db.Exec(`
		UPDATE album_tracks SET position = ?
		WHERE album_id = ? AND track_id = ?`,
		position, albumID, trackID,
	)
	if err != nil {
		return fmt.Errorf("set album track position: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	_, _ = db.Exec("UPDATE albums SET updated_at = datetime('now') WHERE id = ?", albumID)
	return nil
}

// SetAlbumTrackNotes updates per-track notes in an album.
func SetAlbumTrackNotes(db *sql.DB, albumID, trackID, notes string) error {
	res, err := db.Exec(`
		UPDATE album_tracks SET notes = ?
		WHERE album_id = ? AND track_id = ?`,
		notes, albumID, trackID,
	)
	if err != nil {
		return fmt.Errorf("set album track notes: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	_, _ = db.Exec("UPDATE albums SET updated_at = datetime('now') WHERE id = ?", albumID)
	return nil
}

// ReorderAlbumTracks rewrites positions from a full ordered list of track IDs.
// Track IDs not present in the list keep their current position at the end.
func ReorderAlbumTracks(db *sql.DB, albumID string, trackIDs []string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin reorder: %w", err)
	}
	defer tx.Rollback()

	// Reset current positions to a high base so leftover tracks sort after the list.
	if _, err := tx.Exec(`
		UPDATE album_tracks SET position = position + 1000000
		WHERE album_id = ?`, albumID); err != nil {
		return fmt.Errorf("reset positions: %w", err)
	}
	for i, id := range trackIDs {
		if _, err := tx.Exec(`
			UPDATE album_tracks SET position = ?
			WHERE album_id = ? AND track_id = ?`,
			i+1, albumID, id); err != nil {
			return fmt.Errorf("reorder position %d: %w", i, err)
		}
	}
	if _, err := tx.Exec("UPDATE albums SET updated_at = datetime('now') WHERE id = ?", albumID); err != nil {
		return fmt.Errorf("touch album: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reorder: %w", err)
	}
	return nil
}

// unmarshalTags parses the JSON tags column.
func unmarshalTags(raw string, dst *[]string) error {
	if err := json.Unmarshal([]byte(raw), dst); err != nil {
		*dst = []string{}
	}
	return nil
}

// isForeignKeyError reports whether a SQLite error stems from a FK violation.
func isForeignKeyError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "foreign key constraint")
}

// ListTracksByAlbum returns tracks belonging to an album, applying extra
// filters. Convenience wrapper around ListTracks.
func ListTracksByAlbum(db *sql.DB, albumID string, filter models.TrackFilter) ([]models.Track, error) {
	filter.AlbumID = albumID
	return ListTracks(db, filter)
}

// GetAlbumsForTracks returns a map track ID -> the albums it belongs to.
// Batch counterpart of album membership shown in track lists and detail pages.
func GetAlbumsForTracks(db *sql.DB, trackIDs []string) (map[string][]models.Album, error) {
	out := map[string][]models.Album{}
	if len(trackIDs) == 0 {
		return out, nil
	}
	phs := make([]string, len(trackIDs))
	args := make([]any, len(trackIDs))
	for i, id := range trackIDs {
		phs[i] = "?"
		args[i] = id
	}
	rows, err := db.Query(`
		SELECT at.track_id, a.id, a.title, a.kind, a.notes, a.created_at, a.updated_at,
		       (SELECT COUNT(*) FROM album_tracks a2 WHERE a2.album_id = a.id)
		FROM album_tracks at
		JOIN albums a ON a.id = at.album_id
		WHERE at.track_id IN (`+strings.Join(phs, ",")+`)
		ORDER BY a.title COLLATE NOCASE`, args...)
	if err != nil {
		return nil, fmt.Errorf("get albums for tracks: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var trackID string
		a := models.Album{}
		if err := rows.Scan(&trackID, &a.ID, &a.Title, &a.Kind, &a.Notes,
			&a.CreatedAt, &a.UpdatedAt, &a.TrackCount); err != nil {
			return nil, fmt.Errorf("scan album for track: %w", err)
		}
		out[trackID] = append(out[trackID], a)
	}
	return out, rows.Err()
}

// AddTracksToAlbum appends many tracks to an album in a single transaction.
func AddTracksToAlbum(db *sql.DB, albumID string, trackIDs []string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin add tracks to album: %w", err)
	}
	defer tx.Rollback()

	var max int
	if err := tx.QueryRow("SELECT COALESCE(MAX(position), 0) FROM album_tracks WHERE album_id = ?", albumID).Scan(&max); err != nil {
		return fmt.Errorf("read album position: %w", err)
	}
	for _, id := range trackIDs {
		if id == "" {
			continue
		}
		res, err := tx.Exec(`
			INSERT INTO album_tracks (album_id, track_id, position, notes)
			VALUES (?, ?, ?, '')
			ON CONFLICT(album_id, track_id) DO NOTHING`,
			albumID, id, max+1)
		if err != nil {
			if isForeignKeyError(err) {
				return fmt.Errorf("add track %q to album: %w (album or track does not exist)", id, err)
			}
			return fmt.Errorf("add track %q to album: %w", id, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			max++
		}
	}
	if _, err := tx.Exec("UPDATE albums SET updated_at = datetime('now') WHERE id = ?", albumID); err != nil {
		return fmt.Errorf("touch album: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit add tracks to album: %w", err)
	}
	return nil
}
