package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/db"
	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/library"
	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

func TestCatalogPages(t *testing.T) {
	ts, database := newTestServer(t)

	for _, tr := range []*models.Track{
		{ID: "pg-a", Title: "Dawn", IsDownloaded: true},
		{ID: "pg-b", Title: "Dawn", IsDownloaded: true},
		{ID: "pg-c", Title: "Nights", IsDownloaded: false},
	} {
		if err := db.UpsertTrack(database, tr); err != nil {
			t.Fatalf("seed %s: %v", tr.ID, err)
		}
	}
	labels, _ := db.ListLabels(database)
	if len(labels) == 0 {
		t.Fatal("no seeded labels")
	}
	album := mustAPI(t, http.MethodPost, ts.URL+"/api/albums", map[string]any{"title": "Comp", "kind": "compilation"})
	albumID := obj(album)["id"].(string)
	mustAPI(t, http.MethodPost, ts.URL+"/api/albums/"+albumID+"/tracks", map[string]any{"track_id": "pg-a"})

	grp := mustAPI(t, http.MethodPost, ts.URL+"/api/variant-groups", map[string]any{"name": "Dawn variants"})
	groupID := obj(grp)["id"].(string)
	mustAPI(t, http.MethodPost, ts.URL+"/api/variant-groups/"+groupID+"/tracks/pg-a", nil)
	mustAPI(t, http.MethodPost, ts.URL+"/api/variant-groups/"+groupID+"/tracks/pg-b", nil)

	checks := []struct {
		url     string
		markers []string
	}{
		{"/", []string{"Сборники", "Возможные варианты", "track-check", "bulk-bar"}},
		{"/?label=single", []string{"label-chip", "active"}},
		{"/tracks/pg-a", []string{"Метки", "Сборники", "Варианты", "detail-label-sel"}},
		{"/collections/" + albumID, []string{"add-track-sel", "Порядок", "tracklist-row"}},
		{"/variants/" + groupID, []string{"Сравнить варианты", "compare-rendered", "Участники группы"}},
	}
	for _, c := range checks {
		resp, err := http.Get(ts.URL + c.url)
		if err != nil {
			t.Fatalf("GET %s: %v", c.url, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s = %d", c.url, resp.StatusCode)
		}
		for _, m := range c.markers {
			if !strings.Contains(string(body), m) {
				t.Errorf("GET %s: missing %q in body", c.url, m)
			}
		}
	}
}

func TestBulkEndpoints(t *testing.T) {
	ts, database := newTestServer(t)

	for _, tr := range []*models.Track{
		{ID: "bk-a", Title: "One", IsDownloaded: true},
		{ID: "bk-b", Title: "Two", IsDownloaded: true},
		{ID: "bk-c", Title: "Three", IsDownloaded: false},
	} {
		if err := db.UpsertTrack(database, tr); err != nil {
			t.Fatalf("seed %s: %v", tr.ID, err)
		}
	}

	album := mustAPI(t, http.MethodPost, ts.URL+"/api/albums", map[string]any{"title": "Bulk"})
	albumID := obj(album)["id"].(string)
	mustAPI(t, http.MethodPost, ts.URL+"/api/albums/"+albumID+"/tracks/bulk",
		map[string]any{"track_ids": []string{"bk-a", "bk-b", "bk-c"}})
	got := mustAPI(t, http.MethodGet, ts.URL+"/api/albums/"+albumID, nil)
	if len(obj(got)["tracks"].([]any)) != 3 {
		t.Errorf("bulk add album tracks = %v", got)
	}

	label := mustAPI(t, http.MethodPost, ts.URL+"/api/labels", map[string]any{"name": "dance"})
	labelID := obj(label)["id"].(string)
	mustAPI(t, http.MethodPost, ts.URL+"/api/tracks/bulk-labels",
		map[string]any{"track_ids": []string{"bk-a", "bk-b"}, "label_id": labelID})
	tl := mustAPI(t, http.MethodGet, ts.URL+"/api/tracks/bk-a/labels", nil).([]any)
	if len(tl) != 1 || obj(tl[0])["name"] != "dance" {
		t.Errorf("track labels after bulk = %v", tl)
	}
	tl = mustAPI(t, http.MethodGet, ts.URL+"/api/tracks/bk-c/labels", nil).([]any)
	if len(tl) != 0 {
		t.Errorf("bk-c should be unlabeled, got %v", tl)
	}

	// Unknown label -> 404.
	resp, _ := apiJSON(t, http.MethodPost, ts.URL+"/api/tracks/bulk-labels",
		map[string]any{"track_ids": []string{"bk-a"}, "label_id": "nope"})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("bulk label unknown label = %d, want 404", resp.StatusCode)
	}
}

func TestVariantCompare(t *testing.T) {
	ts, database := newTestServer(t)

	for _, tr := range []*models.Track{
		{ID: "cp-a", Title: "Dawn v1", IsDownloaded: true, AudioPath: "/tmp/a.mp3"},
		{ID: "cp-b", Title: "Dawn v2", IsDownloaded: true, AudioPath: "/tmp/b.mp3"},
		{ID: "cp-c", Title: "Dawn v3", IsDownloaded: false},
	} {
		if err := db.UpsertTrack(database, tr); err != nil {
			t.Fatalf("seed %s: %v", tr.ID, err)
		}
	}

	grp := mustAPI(t, http.MethodPost, ts.URL+"/api/variant-groups", map[string]any{"name": "Compare me"})
	groupID := obj(grp)["id"].(string)
	for _, id := range []string{"cp-a", "cp-b", "cp-c"} {
		mustAPI(t, http.MethodPost, ts.URL+"/api/variant-groups/"+groupID+"/tracks/"+id, nil)
	}

	// Seed done analysis results so the compare goroutine skips real analysis.
	for id, score := range map[string]float64{"cp-a": 6.5, "cp-b": 8.7} {
		payload, _ := json.Marshal(map[string]any{
			"status":  "done",
			"results": map[string]any{
				"recommendations": map[string]any{
					"overall_score": score, "mix_quality": "good", "critical_issues": 1,
				},
				"loudness": map[string]any{"lufs_integrated": -13.2, "dynamic_range": 2.4},
			},
		})
		if err := db.UpsertAnalysisResult(database, &db.AnalysisResult{
			TrackID: id, Version: 1, Status: "done", ResultJSON: string(payload),
		}); err != nil {
			t.Fatalf("seed result %s: %v", id, err)
		}
	}

	// Compare only considers downloaded members (cp-a, cp-b).
	resp := mustAPI(t, http.MethodPost, ts.URL+"/api/variant-groups/"+groupID+"/compare", nil)
	if obj(resp)["total"].(float64) != 2 {
		t.Fatalf("compare total = %v, want 2", resp)
	}

	// Both members already "done" -> the background run finishes quickly.
	deadline := time.Now().Add(5 * time.Second)
	var status map[string]any
	for time.Now().Before(deadline) {
		s := mustAPI(t, http.MethodGet, ts.URL+"/api/variant-groups/"+groupID+"/compare-status", nil)
		status = obj(s)
		if status["running"] == false && status["done"] == true {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if status["running"] == true || status["done"] != true {
		t.Fatalf("compare did not finish: %v", status)
	}

	// Two downloaded members analysed + one non-downloaded member listed last.
	rows := status["rows"].([]any)
	if len(rows) != 3 {
		t.Fatalf("rows = %v, want 3", rows)
	}
	first := obj(rows[0])
	if first["track_id"] != "cp-b" {
		t.Errorf("top row = %v, want cp-b (higher score)", first)
	}
	if first["overall_score"].(float64) != 8.7 {
		t.Errorf("top score = %v, want 8.7", first["overall_score"])
	}
	if first["mix_quality"] != "good" {
		t.Errorf("mix quality = %v", first["mix_quality"])
	}
	last := obj(rows[2])
	if last["track_id"] != "cp-c" || last["overall_score"].(float64) != -1 {
		t.Errorf("last row = %v, want cp-c unranked", last)
	}

	// Concurrent second compare while nothing is running is allowed; but a
	// group without downloaded tracks should 400.
	grp2 := mustAPI(t, http.MethodPost, ts.URL+"/api/variant-groups", map[string]any{"name": "No download"})
	group2ID := obj(grp2)["id"].(string)
	mustAPI(t, http.MethodPost, ts.URL+"/api/variant-groups/"+group2ID+"/tracks/cp-c", nil)
	res, _ := apiJSON(t, http.MethodPost, ts.URL+"/api/variant-groups/"+group2ID+"/compare", nil)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("compare no download = %d, want 400", res.StatusCode)
	}

	// Missing group -> 404 on compare-status.
	res, _ = apiJSON(t, http.MethodGet, ts.URL+"/api/variant-groups/missing/compare-status", nil)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("compare-status missing group = %d, want 404", res.StatusCode)
	}
}

// A second compare is rejected while the first is still running. To make the
// check deterministic we short-circuit the background goroutine by pre-seeding
// the server's in-memory compare state (the runner never starts).
func TestVariantCompareDuplicateRejected(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(base+"/data", 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg := &library.Config{Suno: library.SunoConfig{BasePath: base + "/data", AuthToken: "t"}}
	mgr, err := library.NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() { mgr.Close() })
	s, err := NewServer(mgr)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	if err := db.UpsertTrack(mgr.DB, &models.Track{ID: "dup-a", Title: "Dup", IsDownloaded: true, AudioPath: "/tmp/d.mp3"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	grp := &models.VariantGroup{Name: "Dup"}
	if err := db.CreateVariantGroup(mgr.DB, grp); err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := db.AddTrackToGroup(mgr.DB, grp.ID, "dup-a"); err != nil {
		t.Fatalf("add track: %v", err)
	}

	// Simulate an in-flight compare (runner intentionally not started).
	s.compareMu.Lock()
	s.compareState[grp.ID] = &compareState{GroupID: grp.ID, Running: true, Total: 1}
	s.compareMu.Unlock()

	ts := httptest.NewServer(s.Router)
	t.Cleanup(ts.Close)

	// Starting another compare while running -> 409.
	res, _ := apiJSON(t, http.MethodPost, ts.URL+"/api/variant-groups/"+grp.ID+"/compare", nil)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("second compare = %d, want 409", res.StatusCode)
	}
	// Status reflects the in-flight run.
	status := mustAPI(t, http.MethodGet, ts.URL+"/api/variant-groups/"+grp.ID+"/compare-status", nil)
	if obj(status)["running"] != true {
		t.Fatalf("compare-status running = %v, want true", status)
	}
}