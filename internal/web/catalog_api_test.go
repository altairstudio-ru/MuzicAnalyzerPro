package web

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/db"
	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/library"
	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

func newTestServer(t *testing.T) (*httptest.Server, *sql.DB) {
	t.Helper()
	base := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	cfg := &library.Config{
		Suno: library.SunoConfig{
			BasePath:  base,
			AuthToken: "test-token",
		},
	}
	mgr, err := library.NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() { mgr.Close() })

	s, err := NewServer(mgr)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ts := httptest.NewServer(s.Router)
	t.Cleanup(ts.Close)
	return ts, mgr.DB
}

func apiJSON(t *testing.T, method, url string, body any) (*http.Response, any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, url, err)
	}
	defer resp.Body.Close()

	var out any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func mustAPI(t *testing.T, method, url string, body any) any {
	t.Helper()
	resp, out := apiJSON(t, method, url, body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("%s %s: status %d, body=%v", method, url, resp.StatusCode, out)
	}
	return out
}

func obj(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func TestCatalogAPIFlow(t *testing.T) {
	ts, database := newTestServer(t)

	// Seed tracks directly (track ingestion happens through sync).
	for _, tr := range []*models.Track{
		{ID: "trk-a", Title: "Dawn", IsDownloaded: true},
		{ID: "trk-b", Title: "Dawn", IsDownloaded: true},
		{ID: "trk-c", Title: "Nights", IsDownloaded: false},
	} {
		if err := db.UpsertTrack(database, tr); err != nil {
			t.Fatalf("seed track %s: %v", tr.ID, err)
		}
	}

	// --- Albums -------------------------------------------------------------
	mustAPI(t, http.MethodGet, ts.URL+"/api/albums", nil)

	album := mustAPI(t, http.MethodPost, ts.URL+"/api/albums",
		map[string]any{"title": "Neon Nights", "kind": "album", "notes": "debut"})
	albumID := obj(album)["id"].(string)

	// Adding a missing track -> 404.
	resp, _ := apiJSON(t, http.MethodPost, ts.URL+"/api/albums/"+albumID+"/tracks",
		map[string]any{"track_id": "nope"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("add missing track: %d, want 404", resp.StatusCode)
	}

	mustAPI(t, http.MethodPost, ts.URL+"/api/albums/"+albumID+"/tracks",
		map[string]any{"track_id": "trk-a"})
	mustAPI(t, http.MethodPost, ts.URL+"/api/albums/"+albumID+"/tracks",
		map[string]any{"track_id": "trk-c"})

	got := mustAPI(t, http.MethodGet, ts.URL+"/api/albums/"+albumID, nil)
	tracks := obj(got)["tracks"].([]any)
	if len(tracks) != 2 {
		t.Fatalf("album tracks = %d, want 2", len(tracks))
	}
	if obj(tracks[0])["track"].(map[string]any)["id"] != "trk-a" {
		t.Errorf("first track = %v, want trk-a", tracks[0])
	}

	// Reorder -> trk-c first.
	mustAPI(t, http.MethodPost, ts.URL+"/api/albums/"+albumID+"/reorder",
		map[string]any{"track_ids": []string{"trk-c", "trk-a"}})
	got = mustAPI(t, http.MethodGet, ts.URL+"/api/albums/"+albumID, nil)
	tracks = obj(got)["tracks"].([]any)
	if obj(tracks[0])["track"].(map[string]any)["id"] != "trk-c" {
		t.Errorf("after reorder first = %v, want trk-c", tracks[0])
	}

	// Per-track notes + patch album.
	mustAPI(t, http.MethodPatch, ts.URL+"/api/albums/"+albumID+"/tracks/trk-c",
		map[string]any{"notes": "closer"})
	mustAPI(t, http.MethodPatch, ts.URL+"/api/albums/"+albumID,
		map[string]any{"title": "Neon Nights (Deluxe)"})
	got = mustAPI(t, http.MethodGet, ts.URL+"/api/albums/"+albumID, nil)
	if obj(got)["album"].(map[string]any)["title"] != "Neon Nights (Deluxe)" {
		t.Errorf("album title after patch = %v", got)
	}

	mustAPI(t, http.MethodDelete, ts.URL+"/api/albums/"+albumID+"/tracks/trk-a", nil)
	got = mustAPI(t, http.MethodGet, ts.URL+"/api/albums/"+albumID, nil)
	if len(obj(got)["tracks"].([]any)) != 1 {
		t.Errorf("tracks after remove = %v", obj(got)["tracks"])
	}

	// --- Labels -------------------------------------------------------------
	listLabels := mustAPI(t, http.MethodGet, ts.URL+"/api/labels", nil)
	labelList := listLabels.([]any)
	if len(labelList) < len(db.DefaultLabels) {
		t.Errorf("label list = %d entries, want >= %d defaults", len(labelList), len(db.DefaultLabels))
	}

	lbl := mustAPI(t, http.MethodPost, ts.URL+"/api/labels",
		map[string]any{"name": "radio", "color": "#ffb454"})
	labelID := obj(lbl)["id"].(string)

	mustAPI(t, http.MethodPut, ts.URL+"/api/tracks/trk-a/labels",
		map[string]any{"label_ids": []string{labelID}})
	mustAPI(t, http.MethodPut, ts.URL+"/api/tracks/trk-b/labels",
		map[string]any{"label_ids": []string{labelID}})

	tl := mustAPI(t, http.MethodGet, ts.URL+"/api/tracks/trk-a/labels", nil).([]any)
	if len(tl) != 1 {
		t.Fatalf("track labels = %v, want 1", tl)
	}

	// --- Variant groups -----------------------------------------------------
	suggestions := mustAPI(t, http.MethodGet, ts.URL+"/api/variant-groups/suggestions", nil).([]any)
	if len(suggestions) != 1 {
		t.Fatalf("suggestions = %v, want 1 ('Dawn')", suggestions)
	}
	if obj(suggestions[0])["title"] != "Dawn" {
		t.Errorf("suggestion title = %v", suggestions[0])
	}

	grp := mustAPI(t, http.MethodPost, ts.URL+"/api/variant-groups",
		map[string]any{"name": "Dawn variants"})
	groupID := obj(grp)["id"].(string)

	mustAPI(t, http.MethodPost, ts.URL+"/api/variant-groups/"+groupID+"/tracks/trk-a", nil)
	mustAPI(t, http.MethodPost, ts.URL+"/api/variant-groups/"+groupID+"/tracks/trk-b", nil)

	resp, out := apiJSON(t, http.MethodPost, ts.URL+"/api/variant-groups/"+groupID+"/best",
		map[string]any{"track_id": "trk-z"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("best non-member: %d body=%v, want 400", resp.StatusCode, out)
	}
	mustAPI(t, http.MethodPost, ts.URL+"/api/variant-groups/"+groupID+"/best",
		map[string]any{"track_id": "trk-b"})

	detail := mustAPI(t, http.MethodGet, ts.URL+"/api/variant-groups/"+groupID, nil)
	if obj(detail)["group"].(map[string]any)["best_track_id"] != "trk-b" {
		t.Errorf("best track = %v", obj(detail)["group"])
	}
	if len(obj(detail)["tracks"].([]any)) != 2 {
		t.Errorf("group tracks = %v", obj(detail)["tracks"])
	}

	mustAPI(t, http.MethodDelete, ts.URL+"/api/variant-groups/"+groupID+"/tracks/trk-a", nil)
	mustAPI(t, http.MethodDelete, ts.URL+"/api/variant-groups/"+groupID, nil)
}
