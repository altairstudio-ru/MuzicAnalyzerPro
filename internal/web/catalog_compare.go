package web

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/db"
	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

// compareState tracks a background "compare variants" run for one group.
type compareState struct {
	GroupID   string
	Running   bool
	Done      bool
	Total     int
	Processed int
	Current   string
	Error     string
}

// compareRow is a variant member with its quick-analysis ratings.
type compareRow struct {
	TrackID        string  `json:"track_id"`
	Title          string  `json:"title"`
	Artist         string  `json:"artist"`
	IsDownloaded   bool    `json:"is_downloaded"`
	ResultStatus   string  `json:"result_status"`
	OverallScore   float64 `json:"overall_score"`
	MixQuality     string  `json:"mix_quality"`
	CriticalIssues int     `json:"critical_issues"`
	Lufs           float64 `json:"lufs"`
	DynamicRange   float64 `json:"dynamic_range"`
	Best           bool    `json:"best"`
}

// quickMetrics is the fast comparison subset (no whisper) used by
// "Compare variants".
var quickMetrics = []string{"loudness", "phase", "temporal", "spectral", "recommendations"}

// apiCompareVariantGroup kicks off a background quick analysis of all
// downloaded members of a variant group.
func (s *Server) apiCompareVariantGroup(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")
	detail, err := db.GetVariantGroupDetail(s.DB, groupID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if detail == nil {
		writeErr(w, http.StatusNotFound, "variant group not found")
		return
	}

	var members []models.Track
	for _, t := range detail.Tracks {
		if t.IsDownloaded && t.AudioPath != "" {
			members = append(members, t)
		}
	}
	if len(members) == 0 {
		writeErr(w, http.StatusBadRequest, "no downloaded tracks in this group")
		return
	}

	s.compareMu.Lock()
	if st, ok := s.compareState[groupID]; ok && st.Running {
		s.compareMu.Unlock()
		writeErr(w, http.StatusConflict, "comparison already running")
		return
	}
	st := &compareState{GroupID: groupID, Running: true, Total: len(members)}
	s.compareState[groupID] = st
	s.compareMu.Unlock()

	go s.runVariantCompare(groupID, st, members)
	writeJSON(w, http.StatusAccepted, map[string]any{"started": true, "total": len(members)})
}

// runVariantCompare sequentially runs the quick metrics over the members,
// reusing existing "done" analysis results to skip repeated work.
func (s *Server) runVariantCompare(groupID string, st *compareState, members []models.Track) {
	existing, _ := db.GetAnalysisResultsForTracks(s.DB, idsOf(members))

	for _, t := range members {
		s.compareMu.Lock()
		st.Current = t.Title
		s.compareMu.Unlock()

		if ar, ok := existing[t.ID]; ok && ar.Status == "done" {
			// Already analysed (full or quick run) — leave it for ranking.
		} else {
			if _, err := s.Analyzer.Analyze(t.ID, t.AudioPath, quickMetrics, t.Lyrics); err != nil {
				log.Printf("[compare] group %s track %s: %v", groupID, t.ID, err)
				s.compareMu.Lock()
				if st.Error == "" {
					st.Error = err.Error()
				}
				s.compareMu.Unlock()
			}
		}

		s.compareMu.Lock()
		st.Processed++
		s.compareMu.Unlock()
	}

	s.compareMu.Lock()
	st.Running = false
	st.Done = true
	st.Current = ""
	s.compareMu.Unlock()
}

// apiVariantCompareStatus returns the comparison progress together with a
// ranked table of members based on the stored analysis results.
func (s *Server) apiVariantCompareStatus(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")
	rows, err := s.collectCompareRows(groupID)
	if err != nil {
		if err == sql.ErrNoRows {
			writeErr(w, http.StatusNotFound, "variant group not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := map[string]any{
		"running": false,
		"done":    false,
		"total":   0,
		"processed": 0,
		"current": "",
		"error":   "",
		"rows":    rows,
	}
	s.compareMu.Lock()
	if st, ok := s.compareState[groupID]; ok {
		resp["running"] = st.Running
		resp["done"] = st.Done
		resp["total"] = st.Total
		resp["processed"] = st.Processed
		resp["current"] = st.Current
		resp["error"] = st.Error
	}
	s.compareMu.Unlock()

	writeJSON(w, http.StatusOK, resp)
}

// collectCompareRows loads a variant group and ranks its members by overall
// score (missing results rank last).
func (s *Server) collectCompareRows(groupID string) ([]compareRow, error) {
	detail, err := db.GetVariantGroupDetail(s.DB, groupID)
	if err != nil {
		return nil, err
	}
	if detail == nil {
		return nil, sql.ErrNoRows
	}

	results, err := db.GetAnalysisResultsForTracks(s.DB, idsOf(detail.Tracks))
	if err != nil {
		return nil, err
	}

	rows := make([]compareRow, 0, len(detail.Tracks))
	for _, t := range detail.Tracks {
		row := compareRow{
			TrackID:      t.ID,
			Title:        t.Title,
			Artist:       t.Artist,
			IsDownloaded: t.IsDownloaded,
			OverallScore: -1,
			Best:         t.ID == detail.Group.BestTrackID,
		}
		if ar, ok := results[t.ID]; ok {
			row.ResultStatus = ar.Status
			if ar.Status == "done" {
				overall, quality, crit, lufs, dr := parseCompareMetrics(ar.ResultJSON)
				row.OverallScore = overall
				row.MixQuality = quality
				row.CriticalIssues = crit
				row.Lufs = lufs
				row.DynamicRange = dr
			}
		}
		rows = append(rows, row)
	}

	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].OverallScore, rows[j].OverallScore
		if a < 0 {
			return false
		}
		if b < 0 {
			return true
		}
		if a != b {
			return a > b
		}
		return rows[i].Title < rows[j].Title
	})
	return rows, nil
}

// parseCompareMetrics extracts the ranking fields from an analysis result.
func parseCompareMetrics(rawJSON string) (overall float64, quality string, critical int, lufs, dynamicRange float64) {
	overall = -1
	var out struct {
		Results map[string]map[string]any `json:"results"`
		Status  string                    `json:"status"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &out); err != nil {
		return overall, "", 0, 0, 0
	}
	if out.Status != "" && out.Status != "done" {
		return overall, "", 0, 0, 0
	}

	recs := out.Results["recommendations"]
	loud := out.Results["loudness"]
	overall = numOf(recs["overall_score"], -1)
	quality = strOf(recs["mix_quality"])
	critical = intOf(recs["critical_issues"])
	lufs = numOf(loud["lufs_integrated"], 0)
	dynamicRange = numOf(loud["dynamic_range"], 0)
	return overall, quality, critical, lufs, dynamicRange
}

func numOf(v any, fallback float64) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return fallback
}

func strOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func intOf(v any) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 0
}