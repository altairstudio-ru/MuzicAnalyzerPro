package db

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

func mustTrackDB(t *testing.T, d *sql.DB, id, title string, downloaded bool) {
	t.Helper()
	if err := UpsertTrack(d, &models.Track{ID: id, Title: title, IsDownloaded: downloaded}); err != nil {
		t.Fatalf("seed track %s: %v", id, err)
	}
}

func TestGetAlbumsForTracks(t *testing.T) {
	d := newTestDB(t)
	defer d.Close()

	mustTrackDB(t, d, "a1", "One", true)
	mustTrackDB(t, d, "a2", "Two", true)
	mustTrackDB(t, d, "a3", "Three", false)

	alb := &models.Album{Title: "Comp"}
	if err := CreateAlbum(d, alb); err != nil {
		t.Fatalf("CreateAlbum: %v", err)
	}
	if err := AddTracksToAlbum(d, alb.ID, []string{"a1", "a2", "a3"}); err != nil {
		t.Fatalf("AddTracksToAlbum: %v", err)
	}

	m, err := GetAlbumsForTracks(d, []string{"a1", "a2", "nope"})
	if err != nil {
		t.Fatalf("GetAlbumsForTracks: %v", err)
	}
	if len(m["a1"]) != 1 || m["a1"][0].ID != alb.ID {
		t.Errorf("a1 albums = %+v", m["a1"])
	}
	if _, ok := m["nope"]; ok {
		t.Error("unknown track should have no albums")
	}
	// TrackCount reflects members.
	if m["a2"][0].TrackCount != 3 {
		t.Errorf("album track count = %d, want 3", m["a2"][0].TrackCount)
	}
}

func TestGetLabelsForTracks(t *testing.T) {
	d := newTestDB(t)
	defer d.Close()

	for _, tr := range []*models.Track{
		{ID: "l1", Title: "L One", IsDownloaded: true},
		{ID: "l2", Title: "L Two", IsDownloaded: false},
		{ID: "l3", Title: "L Three", IsDownloaded: true},
	} {
		if err := UpsertTrack(d, tr); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	lbl := &models.Label{Name: "radio"}
	if err := CreateLabel(d, lbl); err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	if err := AddLabelToTracks(d, []string{"l1", "l2", "l3"}, lbl.ID); err != nil {
		t.Fatalf("AddLabelToTracks: %v", err)
	}
	// Idempotent.
	if err := AddLabelToTracks(d, []string{"l1"}, lbl.ID); err != nil {
		t.Fatalf("re-add: %v", err)
	}

	m, err := GetLabelsForTracks(d, []string{"l1", "l2", "nope"})
	if err != nil {
		t.Fatalf("GetLabelsForTracks: %v", err)
	}
	if len(m["l1"]) != 1 || m["l1"][0].ID != lbl.ID {
		t.Errorf("l1 labels = %+v", m["l1"])
	}
	if len(m["l2"]) != 1 {
		t.Errorf("l2 labels = %+v", m["l2"])
	}
	if _, ok := m["nope"]; ok {
		t.Error("unknown track should have no labels")
	}
}

func TestGroupsForTrack(t *testing.T) {
	d := newTestDB(t)
	defer d.Close()

	for _, tr := range []*models.Track{
		{ID: "g1", Title: "G One", IsDownloaded: true},
		{ID: "g2", Title: "G Two", IsDownloaded: true},
		{ID: "g3", Title: "G Three", IsDownloaded: true},
	} {
		if err := UpsertTrack(d, tr); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	g := &models.VariantGroup{Name: "G variants"}
	if err := CreateVariantGroup(d, g); err != nil {
		t.Fatalf("CreateVariantGroup: %v", err)
	}
	for _, id := range []string{"g1", "g2"} {
		if err := AddTrackToGroup(d, g.ID, id); err != nil {
			t.Fatalf("add %s: %v", id, err)
		}
	}

	groups, err := GetGroupsForTrack(d, "g1")
	if err != nil {
		t.Fatalf("GetGroupsForTrack: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != g.ID || groups[0].TrackCount != 2 {
		t.Errorf("groups = %+v", groups)
	}
	groups, _ = GetGroupsForTrack(d, "g3")
	if len(groups) != 0 {
		t.Errorf("g3 groups = %+v, want none", groups)
	}
}

func TestGetAnalysisResultsForTracks(t *testing.T) {
	d := newTestDB(t)
	defer d.Close()

	for _, tr := range []*models.Track{
		{ID: "ar1", Title: "A", IsDownloaded: true},
		{ID: "ar2", Title: "B", IsDownloaded: true},
		{ID: "ar3", Title: "C", IsDownloaded: true},
	} {
		if err := UpsertTrack(d, tr); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	payload, _ := json.Marshal(map[string]any{"results": map[string]any{"recommendations": map[string]any{"overall_score": 8.2}}})
	for _, id := range []string{"ar1", "ar2"} {
		if err := UpsertAnalysisResult(d, &AnalysisResult{TrackID: id, Version: 1, Status: "done", ResultJSON: string(payload)}); err != nil {
			t.Fatalf("insert result: %v", err)
		}
	}

	m, err := GetAnalysisResultsForTracks(d, []string{"ar1", "ar2", "ar3"})
	if err != nil {
		t.Fatalf("GetAnalysisResultsForTracks: %v", err)
	}
	if len(m) != 2 {
		t.Fatalf("results = %d (%v), want 2", len(m), m)
	}
	if m["ar1"].Status != "done" {
		t.Errorf("ar1 status = %q", m["ar1"].Status)
	}
}

func TestAddTracksToAlbumReorders(t *testing.T) {
	d := newTestDB(t)
	defer d.Close()

	mustTrackDB(t, d, "r1", "R One", true)
	mustTrackDB(t, d, "r2", "R Two", true)
	mustTrackDB(t, d, "r3", "R Three", true)

	alb := &models.Album{Title: "Ordered"}
	if err := CreateAlbum(d, alb); err != nil {
		t.Fatalf("CreateAlbum: %v", err)
	}
	// First single add, then bulk append.
	if err := AddTrackToAlbum(d, alb.ID, "r1", 0, ""); err != nil {
		t.Fatalf("AddTrackToAlbum: %v", err)
	}
	if err := AddTracksToAlbum(d, alb.ID, []string{"r2", "r3", "r2"}); err != nil {
		t.Fatalf("AddTracksToAlbum bulk: %v", err)
	}

	out, err := GetAlbumWithTracks(d, alb.ID)
	if err != nil {
		t.Fatalf("GetAlbumWithTracks: %v", err)
	}
	got := make([]string, 0, len(out.Tracks))
	for _, it := range out.Tracks {
		got = append(got, it.Track.ID)
	}
	want := []string{"r1", "r2", "r3"}
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pos %d = %s, want %s (order %v)", i, got[i], want[i], got)
		}
	}
}