package db

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

// CreateVariantGroup creates a new variant group. ID is generated if empty.
func CreateVariantGroup(db *sql.DB, g *models.VariantGroup) error {
	if g.ID == "" {
		g.ID = newID("grp")
	}
	if g.Name == "" {
		return fmt.Errorf("variant group name is required")
	}
	_, err := db.Exec(`
		INSERT INTO variant_groups (id, name, notes, best_track_id)
		VALUES (?, ?, ?, ?)`,
		g.ID, g.Name, g.Notes, g.BestTrackID,
	)
	if err != nil {
		return fmt.Errorf("create variant group: %w", err)
	}
	return nil
}

// ListVariantGroups returns all variant groups with member counts.
func ListVariantGroups(db *sql.DB) ([]models.VariantGroup, error) {
	rows, err := db.Query(`
		SELECT g.id, g.name, g.notes, g.best_track_id, g.created_at, g.updated_at,
		       (SELECT COUNT(*) FROM variant_group_tracks gt WHERE gt.group_id = g.id)
		FROM variant_groups g
		ORDER BY g.updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list variant groups: %w", err)
	}
	defer rows.Close()

	groups := []models.VariantGroup{}
	for rows.Next() {
		g := models.VariantGroup{}
		if err := rows.Scan(&g.ID, &g.Name, &g.Notes, &g.BestTrackID,
			&g.CreatedAt, &g.UpdatedAt, &g.TrackCount); err != nil {
			return nil, fmt.Errorf("scan variant group: %w", err)
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// GetVariantGroupDetail returns a variant group with its member tracks.
// Returns nil, nil when the group does not exist.
func GetVariantGroupDetail(db *sql.DB, id string) (*models.VariantGroupDetail, error) {
	row := db.QueryRow(`
		SELECT id, name, notes, best_track_id, created_at, updated_at
		FROM variant_groups WHERE id = ?`, id)

	out := &models.VariantGroupDetail{}
	err := row.Scan(&out.Group.ID, &out.Group.Name, &out.Group.Notes,
		&out.Group.BestTrackID, &out.Group.CreatedAt, &out.Group.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get variant group: %w", err)
	}

	rows, err := db.Query(`
		SELECT t.id, t.title, t.artist, t.prompt, t.lyrics, t.tags, t.workspace,
		       t.duration, t.created_at, t.audio_path, t.audio_hash,
		       t.lyrics_path, t.is_downloaded, t.file_size
		FROM variant_group_tracks gt
		JOIN tracks t ON t.id = gt.track_id
		WHERE gt.group_id = ?
		ORDER BY t.created_at DESC`, id)
	if err != nil {
		return nil, fmt.Errorf("get variant group tracks: %w", err)
	}
	defer rows.Close()

	out.Tracks = []models.Track{}
	for rows.Next() {
		t := models.Track{}
		var tagsJSON string
		var dl int
		if err := rows.Scan(&t.ID, &t.Title, &t.Artist, &t.Prompt, &t.Lyrics,
			&tagsJSON, &t.Workspace, &t.Duration, &t.CreatedAt, &t.AudioPath,
			&t.AudioHash, &t.LyricsPath, &dl, &t.FileSize); err != nil {
			return nil, fmt.Errorf("scan variant track: %w", err)
		}
		t.IsDownloaded = dl == 1
		if err := unmarshalTags(tagsJSON, &t.Tags); err != nil {
			return nil, err
		}
		out.Tracks = append(out.Tracks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("variant tracks rows: %w", err)
	}

	out.Group.TrackCount = len(out.Tracks)
	return out, nil
}

// UpdateVariantGroup updates name/notes of a variant group.
func UpdateVariantGroup(db *sql.DB, id, name, notes string) error {
	res, err := db.Exec(`
		UPDATE variant_groups SET name = ?, notes = ?, updated_at = datetime('now')
		WHERE id = ?`, name, notes, id)
	if err != nil {
		return fmt.Errorf("update variant group: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteVariantGroup removes a variant group (membership cascades away).
func DeleteVariantGroup(db *sql.DB, id string) error {
	res, err := db.Exec("DELETE FROM variant_groups WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete variant group: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// AddTrackToGroup adds a track to a variant group (idempotent).
func AddTrackToGroup(db *sql.DB, groupID, trackID string) error {
	_, err := db.Exec(`
		INSERT OR IGNORE INTO variant_group_tracks (group_id, track_id)
		VALUES (?, ?)`, groupID, trackID)
	if err != nil {
		if isForeignKeyError(err) {
			return fmt.Errorf("add track to variant group: %w (group or track does not exist)", err)
		}
		return fmt.Errorf("add track to variant group: %w", err)
	}
	_, _ = db.Exec("UPDATE variant_groups SET updated_at = datetime('now') WHERE id = ?", groupID)
	return nil
}

// RemoveTrackFromGroup removes a track from a variant group.
func RemoveTrackFromGroup(db *sql.DB, groupID, trackID string) error {
	res, err := db.Exec(`
		DELETE FROM variant_group_tracks WHERE group_id = ? AND track_id = ?`,
		groupID, trackID)
	if err != nil {
		return fmt.Errorf("remove track from variant group: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}

	// Clear best_track_id if it pointed at the removed track.
	if _, err := db.Exec(`
		UPDATE variant_groups SET best_track_id = '', updated_at = datetime('now')
		WHERE id = ? AND best_track_id = ?`, groupID, trackID); err != nil {
		return fmt.Errorf("reset best track: %w", err)
	}
	_, _ = db.Exec("UPDATE variant_groups SET updated_at = datetime('now') WHERE id = ?", groupID)
	return nil
}

// SetBestTrack marks a member track as the best variant of the group.
// An empty trackID clears the selection. The track must be a member.
func SetBestTrack(db *sql.DB, groupID, trackID string) error {
	if trackID != "" {
		var found int
		if err := db.QueryRow(`
			SELECT COUNT(*) FROM variant_group_tracks
			WHERE group_id = ? AND track_id = ?`, groupID, trackID).Scan(&found); err != nil {
			return fmt.Errorf("check variant membership: %w", err)
		}
		if found == 0 {
			return fmt.Errorf("track %q is not a member of variant group %q", trackID, groupID)
		}
	}
	res, err := db.Exec(`
		UPDATE variant_groups SET best_track_id = ?, updated_at = datetime('now')
		WHERE id = ?`, trackID, groupID)
	if err != nil {
		return fmt.Errorf("set best track: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SuggestVariantGroups finds groups of tracks sharing the same title — a hint
// that they might be alternative versions of one song. Only titles with at
// least two distinct track IDs are reported.
func SuggestVariantGroups(db *sql.DB) ([]models.VariantSuggestion, error) {
	rows, err := db.Query(`
		SELECT title, COUNT(*) AS cnt,
		       SUM(CASE WHEN is_downloaded = 1 THEN 1 ELSE 0 END) AS dl,
		       GROUP_CONCAT(id)
		FROM tracks
		WHERE title != ''
		GROUP BY title
		HAVING cnt >= 2
		ORDER BY title COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("suggest variant groups: %w", err)
	}
	defer rows.Close()

	out := []models.VariantSuggestion{}
	for rows.Next() {
		var title, ids string
		var cnt, dl int
		if err := rows.Scan(&title, &cnt, &dl, &ids); err != nil {
			return nil, fmt.Errorf("scan suggestion: %w", err)
		}
		s := models.VariantSuggestion{
			Title:         title,
			AllDownloaded: cnt > 0 && dl == cnt,
		}
		for _, id := range strings.Split(ids, ",") {
			if id != "" {
				s.TrackIDs = append(s.TrackIDs, id)
			}
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetGroupsForTrack returns the variant groups a track belongs to, with counts.
func GetGroupsForTrack(db *sql.DB, trackID string) ([]models.VariantGroup, error) {
	rows, err := db.Query(`
		SELECT g.id, g.name, g.notes, g.best_track_id, g.created_at, g.updated_at,
		       (SELECT COUNT(*) FROM variant_group_tracks gt WHERE gt.group_id = g.id)
		FROM variant_group_tracks vgt
		JOIN variant_groups g ON g.id = vgt.group_id
		WHERE vgt.track_id = ?
		ORDER BY g.updated_at DESC`, trackID)
	if err != nil {
		return nil, fmt.Errorf("get groups for track: %w", err)
	}
	defer rows.Close()

	groups := []models.VariantGroup{}
	for rows.Next() {
		g := models.VariantGroup{}
		if err := rows.Scan(&g.ID, &g.Name, &g.Notes, &g.BestTrackID,
			&g.CreatedAt, &g.UpdatedAt, &g.TrackCount); err != nil {
			return nil, fmt.Errorf("scan group for track: %w", err)
		}
		groups = append(groups, g)
	}
	if groups == nil {
		groups = []models.VariantGroup{}
	}
	return groups, rows.Err()
}
