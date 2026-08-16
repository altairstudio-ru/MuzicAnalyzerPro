package db

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

// DefaultLabels are seeded on first startup so the user can immediately start
// tagging tracks. Colors follow the app amber/dark palette.
var DefaultLabels = []models.Label{
	{Name: "single", Color: "#ffb454"},
	{Name: "album", Color: "#a18cff"},
	{Name: "compilation", Color: "#6ee7a0"},
	{Name: "draft", Color: "#8f8a83"},
	{Name: "final", Color: "#e5b567"},
	{Name: "b-side", Color: "#f0615f"},
	{Name: "cover", Color: "#b9a8ff"},
	{Name: "remix", Color: "#e5a03f"},
}

// EnsureDefaultLabels inserts default curation labels if the table is empty.
func EnsureDefaultLabels(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM labels").Scan(&count); err != nil {
		return fmt.Errorf("count labels: %w", err)
	}
	if count > 0 {
		return nil
	}
	for _, l := range DefaultLabels {
		id := newID("lbl")
		if _, err := db.Exec(`
			INSERT OR IGNORE INTO labels (id, name, color) VALUES (?, ?, ?)`,
			id, l.Name, l.Color); err != nil {
			return fmt.Errorf("insert default label %q: %w", l.Name, err)
		}
	}
	return nil
}

// CreateLabel inserts a new label. The ID is generated if empty.
func CreateLabel(db *sql.DB, l *models.Label) error {
	if l.ID == "" {
		l.ID = newID("lbl")
	}
	if l.Name == "" {
		return fmt.Errorf("label name is required")
	}
	if l.Color == "" {
		l.Color = "#ffb454"
	}
	_, err := db.Exec("INSERT INTO labels (id, name, color) VALUES (?, ?, ?)",
		l.ID, l.Name, l.Color)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
			return fmt.Errorf("label %q already exists", l.Name)
		}
		return fmt.Errorf("create label: %w", err)
	}
	return nil
}

// ListLabels returns all labels ordered by name with usage counts.
func ListLabels(db *sql.DB) ([]models.Label, error) {
	rows, err := db.Query(`
		SELECT l.id, l.name, l.color, l.created_at,
		       (SELECT COUNT(*) FROM track_labels tl WHERE tl.label_id = l.id)
		FROM labels l
		ORDER BY l.name COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("list labels: %w", err)
	}
	defer rows.Close()

	labels := []models.Label{}
	for rows.Next() {
		l := models.Label{}
		if err := rows.Scan(&l.ID, &l.Name, &l.Color, &l.CreatedAt, &l.TrackCount); err != nil {
			return nil, fmt.Errorf("scan label: %w", err)
		}
		labels = append(labels, l)
	}
	return labels, rows.Err()
}

// GetLabel returns a single label by ID, or nil when missing.
func GetLabel(db *sql.DB, id string) (*models.Label, error) {
	row := db.QueryRow(`
		SELECT l.id, l.name, l.color, l.created_at,
		       (SELECT COUNT(*) FROM track_labels tl WHERE tl.label_id = l.id)
		FROM labels l WHERE l.id = ?`, id)
	l := &models.Label{}
	err := row.Scan(&l.ID, &l.Name, &l.Color, &l.CreatedAt, &l.TrackCount)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get label: %w", err)
	}
	return l, nil
}

// UpdateLabel updates a label's name and/or color (empty string keeps current).
func UpdateLabel(db *sql.DB, id, name, color string) error {
	if name != "" && color != "" {
		_, err := db.Exec("UPDATE labels SET name = ?, color = ? WHERE id = ?", name, color, id)
		if err != nil {
			return fmt.Errorf("update label: %w", err)
		}
		return nil
	}
	if name != "" {
		_, err := db.Exec("UPDATE labels SET name = ? WHERE id = ?", name, id)
		if err != nil {
			return fmt.Errorf("update label name: %w", err)
		}
		return nil
	}
	if color != "" {
		_, err := db.Exec("UPDATE labels SET color = ? WHERE id = ?", color, id)
		if err != nil {
			return fmt.Errorf("update label color: %w", err)
		}
	}
	return nil
}

// DeleteLabel removes a label (assignments cascade away).
func DeleteLabel(db *sql.DB, id string) error {
	res, err := db.Exec("DELETE FROM labels WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete label: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetTrackLabels replaces the full label set of a track.
func SetTrackLabels(db *sql.DB, trackID string, labelIDs []string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin set labels: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM track_labels WHERE track_id = ?", trackID); err != nil {
		return fmt.Errorf("clear track labels: %w", err)
	}
	for _, id := range labelIDs {
		if id == "" {
			continue
		}
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO track_labels (track_id, label_id) VALUES (?, ?)`,
			trackID, id); err != nil {
			if isForeignKeyError(err) {
				return fmt.Errorf("set track labels: label %q does not exist", id)
			}
			return fmt.Errorf("insert track label %q: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit set labels: %w", err)
	}
	return nil
}

// GetTrackLabels returns the labels assigned to a track.
func GetTrackLabels(db *sql.DB, trackID string) ([]models.Label, error) {
	rows, err := db.Query(`
		SELECT l.id, l.name, l.color, l.created_at,
		       (SELECT COUNT(*) FROM track_labels t2 WHERE t2.label_id = l.id)
		FROM track_labels tl
		JOIN labels l ON l.id = tl.label_id
		WHERE tl.track_id = ?
		ORDER BY l.name COLLATE NOCASE`, trackID)
	if err != nil {
		return nil, fmt.Errorf("get track labels: %w", err)
	}
	defer rows.Close()

	labels := []models.Label{}
	for rows.Next() {
		l := models.Label{}
		if err := rows.Scan(&l.ID, &l.Name, &l.Color, &l.CreatedAt, &l.TrackCount); err != nil {
			return nil, fmt.Errorf("scan track label: %w", err)
		}
		labels = append(labels, l)
	}
	return labels, rows.Err()
}

// ListTracksByLabel returns tracks carrying a given label name.
// Convenience wrapper around ListTracks.
func ListTracksByLabel(db *sql.DB, label string, filter models.TrackFilter) ([]models.Track, error) {
	filter.Label = label
	return ListTracks(db, filter)
}

// GetLabelsForTracks returns a map track ID -> labels assigned to it.
// Batch counterpart of GetTrackLabels, used for list pages.
func GetLabelsForTracks(db *sql.DB, trackIDs []string) (map[string][]models.Label, error) {
	out := map[string][]models.Label{}
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
		SELECT tl.track_id, l.id, l.name, l.color, l.created_at,
		       (SELECT COUNT(*) FROM track_labels t2 WHERE t2.label_id = l.id)
		FROM track_labels tl
		JOIN labels l ON l.id = tl.label_id
		WHERE tl.track_id IN (`+strings.Join(phs, ",")+`)
		ORDER BY l.name COLLATE NOCASE`, args...)
	if err != nil {
		return nil, fmt.Errorf("get labels for tracks: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var trackID string
		l := models.Label{}
		if err := rows.Scan(&trackID, &l.ID, &l.Name, &l.Color, &l.CreatedAt, &l.TrackCount); err != nil {
			return nil, fmt.Errorf("scan label for track: %w", err)
		}
		out[trackID] = append(out[trackID], l)
	}
	return out, rows.Err()
}

// AddLabelToTracks appends (idempotently) a label to many tracks at once.
func AddLabelToTracks(db *sql.DB, trackIDs []string, labelID string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin add label to tracks: %w", err)
	}
	defer tx.Rollback()

	for _, id := range trackIDs {
		if id == "" {
			continue
		}
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO track_labels (track_id, label_id) VALUES (?, ?)`,
			id, labelID); err != nil {
			if isForeignKeyError(err) {
				return fmt.Errorf("add label to tracks: track or label does not exist")
			}
			return fmt.Errorf("add label %q to track %q: %w", labelID, id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit add label to tracks: %w", err)
	}
	return nil
}
