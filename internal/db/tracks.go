package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

const trackSelectCols = `id, title, artist, prompt, lyrics, tags, workspace,
	duration, created_at, audio_path, audio_hash,
	lyrics_path, is_downloaded, file_size,
	upvote_count, play_count, is_liked, track_type, model_name`

const trackSelectColsPrefixed = `t.id, t.title, t.artist, t.prompt, t.lyrics, t.tags, t.workspace,
	t.duration, t.created_at, t.audio_path, t.audio_hash,
	t.lyrics_path, t.is_downloaded, t.file_size,
	t.upvote_count, t.play_count, t.is_liked, t.track_type, t.model_name`

func scanTrack(scan func(dest ...any) error) (models.Track, error) {
	t := models.Track{}
	var tagsJSON string
	var dl, liked int
	err := scan(&t.ID, &t.Title, &t.Artist, &t.Prompt, &t.Lyrics,
		&tagsJSON, &t.Workspace, &t.Duration, &t.CreatedAt,
		&t.AudioPath, &t.AudioHash, &t.LyricsPath, &dl, &t.FileSize,
		&t.UpvoteCount, &t.PlayCount, &liked, &t.TrackType, &t.ModelName)
	if err != nil {
		return t, err
	}
	t.IsDownloaded = dl == 1
	t.IsLiked = liked == 1
	if err := json.Unmarshal([]byte(tagsJSON), &t.Tags); err != nil {
		t.Tags = []string{}
	}
	return t, nil
}

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
	liked := 0
	if t.IsLiked {
		liked = 1
	}

	_, err = db.Exec(`
		INSERT INTO tracks (id, title, artist, prompt, lyrics, tags, workspace,
		                    duration, created_at, audio_path, audio_hash,
		                    lyrics_path, is_downloaded, file_size,
		                    upvote_count, play_count, is_liked, track_type, model_name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			lyrics_path   = excluded.lyrics_path,
			is_downloaded = excluded.is_downloaded,
			file_size     = excluded.file_size,
			upvote_count  = excluded.upvote_count,
			play_count    = excluded.play_count,
			is_liked      = excluded.is_liked,
			track_type    = excluded.track_type,
			model_name    = excluded.model_name,
			updated_at    = datetime('now')`,
		t.ID, t.Title, t.Artist, t.Prompt, t.Lyrics, string(tagsJSON),
		t.Workspace, t.Duration, t.CreatedAt, t.AudioPath, t.AudioHash,
		t.LyricsPath, dl, t.FileSize,
		t.UpvoteCount, t.PlayCount, liked, t.TrackType, t.ModelName,
	)
	if err != nil {
		return fmt.Errorf("upsert track: %w", err)
	}
	return nil
}

// GetTrack retrieves a single track by its ID.
func GetTrack(db *sql.DB, id string) (*models.Track, error) {
	row := db.QueryRow(`SELECT `+trackSelectCols+` FROM tracks WHERE id = ?`, id)
	t, err := scanTrack(row.Scan)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get track: %w", err)
	}
	return &t, nil
}

// ListTracks returns tracks matching the given filter criteria.
func ListTracks(db *sql.DB, filter models.TrackFilter) ([]models.Track, error) {
	var conditions []string
	var args []interface{}
	joins := ""

	if filter.Workspace != "" {
		conditions = append(conditions, "t.workspace = ?")
		args = append(args, filter.Workspace)
	}
	if filter.Tag != "" {
		conditions = append(conditions, "t.tags LIKE ?")
		args = append(args, "%"+filter.Tag+"%")
	}
	if filter.AlbumID != "" {
		joins += " JOIN album_tracks at ON at.track_id = t.id"
		conditions = append(conditions, "at.album_id = ?")
		args = append(args, filter.AlbumID)
	}
	if filter.Label != "" {
		joins += " JOIN track_labels tl ON tl.track_id = t.id" +
			" JOIN labels lb ON lb.id = tl.label_id"
		conditions = append(conditions, "lb.name = ?")
		args = append(args, filter.Label)
	}
	if filter.TrackType != "" {
		conditions = append(conditions, "t.track_type = ?")
		args = append(args, filter.TrackType)
	}
	if filter.Search != "" {
		conditions = append(conditions,
			"(t.title LIKE ? OR t.prompt LIKE ? OR t.lyrics LIKE ?)")
		s := "%" + filter.Search + "%"
		args = append(args, s, s, s)
	}
	if filter.Downloaded != nil {
		v := 0
		if *filter.Downloaded {
			v = 1
		}
		conditions = append(conditions, "t.is_downloaded = ?")
		args = append(args, v)
	}

	query := "SELECT " + trackSelectColsPrefixed + " FROM tracks t" + joins
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY t.created_at DESC"

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
		t, err := scanTrack(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan track: %w", err)
		}
		tracks = append(tracks, t)
	}
	if tracks == nil {
		tracks = []models.Track{}
	}
	return tracks, rows.Err()
}

// GetAllTracks returns every track in the database.
func GetAllTracks(db *sql.DB) ([]models.Track, error) {
	rows, err := db.Query("SELECT " + trackSelectCols + " FROM tracks")
	if err != nil {
		return nil, fmt.Errorf("get all tracks: %w", err)
	}
	defer rows.Close()

	var tracks []models.Track
	for rows.Next() {
		t, err := scanTrack(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan track: %w", err)
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
