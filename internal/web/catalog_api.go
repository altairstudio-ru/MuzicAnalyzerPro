package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/db"
	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[api] encode response: %v", err)
	}
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func isNotFound(err error) bool {
	return err == sql.ErrNoRows || errors.Is(err, sql.ErrNoRows)
}

// ---------------------------------------------------------------------------
// Albums / collections

func (s *Server) apiListAlbums(w http.ResponseWriter, r *http.Request) {
	albums, err := db.ListAlbums(s.DB)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, albums)
}

func (s *Server) apiCreateAlbum(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
		Kind  string `json:"kind"`
		Notes string `json:"notes"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	a := &models.Album{Title: strings.TrimSpace(req.Title), Kind: req.Kind, Notes: req.Notes}
	if err := db.CreateAlbum(s.DB, a); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) apiGetAlbum(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := db.GetAlbumWithTracks(s.DB, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if out == nil {
		writeErr(w, http.StatusNotFound, "album not found")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) apiUpdateAlbum(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Title *string `json:"title"`
		Kind  *string `json:"kind"`
		Notes *string `json:"notes"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	existing, err := db.GetAlbumWithTracks(s.DB, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeErr(w, http.StatusNotFound, "album not found")
		return
	}
	title, kind, notes := existing.Album.Title, existing.Album.Kind, existing.Album.Notes
	if req.Title != nil {
		title = *req.Title
	}
	if req.Kind != nil {
		kind = *req.Kind
	}
	if req.Notes != nil {
		notes = *req.Notes
	}
	if err := db.UpdateAlbum(s.DB, id, title, kind, notes); err != nil {
		if isNotFound(err) {
			writeErr(w, http.StatusNotFound, "album not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

func (s *Server) apiDeleteAlbum(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := db.DeleteAlbum(s.DB, id); err != nil {
		if isNotFound(err) {
			writeErr(w, http.StatusNotFound, "album not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) apiAddAlbumTrack(w http.ResponseWriter, r *http.Request) {
	albumID := chi.URLParam(r, "id")
	var req struct {
		TrackID  string `json:"track_id"`
		Position int    `json:"position"`
		Notes    string `json:"notes"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.TrackID == "" {
		writeErr(w, http.StatusBadRequest, "track_id is required")
		return
	}
	tr, err := db.GetTrack(s.DB, req.TrackID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tr == nil {
		writeErr(w, http.StatusNotFound, "track not found")
		return
	}
	if err := db.AddTrackToAlbum(s.DB, albumID, req.TrackID, req.Position, req.Notes); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"added": true})
}

func (s *Server) apiRemoveAlbumTrack(w http.ResponseWriter, r *http.Request) {
	albumID := chi.URLParam(r, "id")
	trackID := chi.URLParam(r, "track_id")
	if err := db.RemoveTrackFromAlbum(s.DB, albumID, trackID); err != nil {
		if isNotFound(err) {
			writeErr(w, http.StatusNotFound, "track not in album")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"removed": true})
}

func (s *Server) apiUpdateAlbumTrack(w http.ResponseWriter, r *http.Request) {
	albumID := chi.URLParam(r, "id")
	trackID := chi.URLParam(r, "track_id")
	var req struct {
		Position *int    `json:"position"`
		Notes    *string `json:"notes"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Position != nil {
		if err := db.SetAlbumTrackPosition(s.DB, albumID, trackID, *req.Position); err != nil {
			if isNotFound(err) {
				writeErr(w, http.StatusNotFound, "track not in album")
				return
			}
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if req.Notes != nil {
		if err := db.SetAlbumTrackNotes(s.DB, albumID, trackID, *req.Notes); err != nil {
			if isNotFound(err) {
				writeErr(w, http.StatusNotFound, "track not in album")
				return
			}
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

func (s *Server) apiReorderAlbumTracks(w http.ResponseWriter, r *http.Request) {
	albumID := chi.URLParam(r, "id")
	var req struct {
		TrackIDs []string `json:"track_ids"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := db.ReorderAlbumTracks(s.DB, albumID, req.TrackIDs); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"reordered": true})
}

func (s *Server) apiBulkAddAlbumTracks(w http.ResponseWriter, r *http.Request) {
	albumID := chi.URLParam(r, "id")
	var req struct {
		TrackIDs []string `json:"track_ids"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if albumID == "" {
		writeErr(w, http.StatusBadRequest, "album id is required")
		return
	}
	existing, err := db.GetAlbumWithTracks(s.DB, albumID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeErr(w, http.StatusNotFound, "album not found")
		return
	}
	if err := db.AddTracksToAlbum(s.DB, albumID, req.TrackIDs); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"added": len(req.TrackIDs)})
}

// ---------------------------------------------------------------------------
// Labels

func (s *Server) apiListLabels(w http.ResponseWriter, r *http.Request) {
	labels, err := db.ListLabels(s.DB)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, labels)
}

func (s *Server) apiCreateLabel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	l := &models.Label{Name: strings.TrimSpace(req.Name), Color: req.Color}
	if err := db.CreateLabel(s.DB, l); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (s *Server) apiUpdateLabel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "label_id")
	var req struct {
		Name  *string `json:"name"`
		Color *string `json:"color"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	name, color := "", ""
	if req.Name != nil {
		name = *req.Name
	}
	if req.Color != nil {
		color = *req.Color
	}
	if err := db.UpdateLabel(s.DB, id, name, color); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

func (s *Server) apiDeleteLabel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "label_id")
	if err := db.DeleteLabel(s.DB, id); err != nil {
		if isNotFound(err) {
			writeErr(w, http.StatusNotFound, "label not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) apiGetTrackLabels(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	labels, err := db.GetTrackLabels(s.DB, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, labels)
}

func (s *Server) apiSetTrackLabels(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		LabelIDs []string `json:"label_ids"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := db.SetTrackLabels(s.DB, id, req.LabelIDs); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

func (s *Server) apiBulkSetLabels(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TrackIDs []string `json:"track_ids"`
		LabelID  string   `json:"label_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.LabelID == "" {
		writeErr(w, http.StatusBadRequest, "label_id is required")
		return
	}
	label, err := db.GetLabel(s.DB, req.LabelID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if label == nil {
		writeErr(w, http.StatusNotFound, "label not found")
		return
	}
	if err := db.AddLabelToTracks(s.DB, req.TrackIDs, req.LabelID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"labeled": len(req.TrackIDs)})
}

// ---------------------------------------------------------------------------
// Variant groups

func (s *Server) apiListVariantGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := db.ListVariantGroups(s.DB)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

func (s *Server) apiCreateVariantGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Notes string `json:"notes"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	g := &models.VariantGroup{Name: strings.TrimSpace(req.Name), Notes: req.Notes}
	if err := db.CreateVariantGroup(s.DB, g); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (s *Server) apiGetVariantGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := db.GetVariantGroupDetail(s.DB, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if out == nil {
		writeErr(w, http.StatusNotFound, "variant group not found")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) apiUpdateVariantGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Name  *string `json:"name"`
		Notes *string `json:"notes"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	existing, err := db.GetVariantGroupDetail(s.DB, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeErr(w, http.StatusNotFound, "variant group not found")
		return
	}
	name, notes := existing.Group.Name, existing.Group.Notes
	if req.Name != nil {
		name = *req.Name
	}
	if req.Notes != nil {
		notes = *req.Notes
	}
	if err := db.UpdateVariantGroup(s.DB, id, name, notes); err != nil {
		if isNotFound(err) {
			writeErr(w, http.StatusNotFound, "variant group not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

func (s *Server) apiDeleteVariantGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := db.DeleteVariantGroup(s.DB, id); err != nil {
		if isNotFound(err) {
			writeErr(w, http.StatusNotFound, "variant group not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) apiAddVariantTrack(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")
	trackID := chi.URLParam(r, "track_id")
	tr, err := db.GetTrack(s.DB, trackID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tr == nil {
		writeErr(w, http.StatusNotFound, "track not found")
		return
	}
	if err := db.AddTrackToGroup(s.DB, groupID, trackID); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"added": true})
}

func (s *Server) apiRemoveVariantTrack(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")
	trackID := chi.URLParam(r, "track_id")
	if err := db.RemoveTrackFromGroup(s.DB, groupID, trackID); err != nil {
		if isNotFound(err) {
			writeErr(w, http.StatusNotFound, "track not in group")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"removed": true})
}

func (s *Server) apiSetBestTrack(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")
	var req struct {
		TrackID string `json:"track_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := db.SetBestTrack(s.DB, groupID, req.TrackID); err != nil {
		if isNotFound(err) {
			writeErr(w, http.StatusNotFound, "variant group not found")
			return
		}
		if strings.Contains(err.Error(), "not a member") {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"best_set": true})
}

func (s *Server) apiVariantSuggestions(w http.ResponseWriter, r *http.Request) {
	suggestions, err := db.SuggestVariantGroups(s.DB)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, suggestions)
}
