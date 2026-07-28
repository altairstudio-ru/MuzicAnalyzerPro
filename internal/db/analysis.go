package db

import (
	"database/sql"
	"fmt"
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
	err := row.Scan(&r.TrackID, &r.Version, &r.Status, &r.ErrorMsg, &r.ResultJSON, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get analysis result: %w", err)
	}
	return r, nil
}

func DeleteAnalysisResult(db *sql.DB, trackID string) error {
	_, err := db.Exec("DELETE FROM analysis_results WHERE track_id = ?", trackID)
	if err != nil {
		return fmt.Errorf("delete analysis result: %w", err)
	}
	return nil
}
