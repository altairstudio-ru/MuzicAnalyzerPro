package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type AnalysisResult struct {
	TrackID    string    `json:"track_id"`
	Version    int       `json:"version"`
	Status     string    `json:"status"`
	ErrorMsg   string    `json:"error_msg,omitempty"`
	ResultJSON string    `json:"result_json"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func UpsertAnalysisResult(db *sql.DB, r *AnalysisResult) error {
	_, err := db.Exec(`
		INSERT INTO analysis_results (track_id, version, status, error_msg, result_json, updated_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(track_id) DO UPDATE SET
			version     = excluded.version,
			status      = excluded.status,
			error_msg   = excluded.error_msg,
			result_json = excluded.result_json,
			updated_at  = datetime('now')`,
		r.TrackID, r.Version, r.Status, r.ErrorMsg, r.ResultJSON,
	)
	if err != nil {
		return fmt.Errorf("upsert analysis result: %w", err)
	}
	return nil
}

func GetAnalysisResult(db *sql.DB, trackID string) (*AnalysisResult, error) {
	row := db.QueryRow(`
		SELECT track_id, version, status, error_msg, result_json, created_at, updated_at
		FROM analysis_results WHERE track_id = ?`, trackID)

	r := &AnalysisResult{}
	var created, updated string
	err := row.Scan(&r.TrackID, &r.Version, &r.Status, &r.ErrorMsg, &r.ResultJSON, &created, &updated)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get analysis result: %w", err)
	}
	r.CreatedAt = parseSQLiteTime(created)
	r.UpdatedAt = parseSQLiteTime(updated)
	return r, nil
}

func DeleteAnalysisResult(db *sql.DB, trackID string) error {
	_, err := db.Exec("DELETE FROM analysis_results WHERE track_id = ?", trackID)
	if err != nil {
		return fmt.Errorf("delete analysis result: %w", err)
	}
	return nil
}

// GetAnalysisResultsForTracks returns the latest analysis result for each of
// the given tracks (map track ID -> result), skipping tracks without one.
func GetAnalysisResultsForTracks(db *sql.DB, trackIDs []string) (map[string]*AnalysisResult, error) {
	out := map[string]*AnalysisResult{}
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
		SELECT track_id, version, status, error_msg, result_json, created_at, updated_at
		FROM analysis_results WHERE track_id IN (`+strings.Join(phs, ",")+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("get analysis results for tracks: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		r := &AnalysisResult{}
		var created, updated string
		if err := rows.Scan(&r.TrackID, &r.Version, &r.Status, &r.ErrorMsg,
			&r.ResultJSON, &created, &updated); err != nil {
			return nil, fmt.Errorf("scan analysis result: %w", err)
		}
		r.CreatedAt = parseSQLiteTime(created)
		r.UpdatedAt = parseSQLiteTime(updated)
		out[r.TrackID] = r
	}
	return out, rows.Err()
}

// sqliteTime is the format used by SQLite's datetime('now').
const sqliteTime = "2006-01-02 15:04:05"

// parseSQLiteTime parses SQLite TEXT timestamps into a Go time value.
// Falls back to the zero time when the value does not parse.
func parseSQLiteTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{sqliteTime, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
