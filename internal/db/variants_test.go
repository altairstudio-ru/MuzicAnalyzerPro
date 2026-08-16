package db

import (
	"testing"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

func TestVariantGroupLifecycle(t *testing.T) {
	d := newTestDB(t)
	defer d.Close()

	mustTrack(t, d, "v1", "Nights (take 1)", true)
	mustTrack(t, d, "v2", "Nights (take 2)", true)
	mustTrack(t, d, "v3", "Nights (final)", true)

	g := &models.VariantGroup{Name: "Nights"}
	if err := CreateVariantGroup(d, g); err != nil {
		t.Fatalf("CreateVariantGroup: %v", err)
	}
	if g.ID == "" {
		t.Fatal("CreateVariantGroup did not generate ID")
	}

	for _, id := range []string{"v1", "v2", "v3"} {
		if err := AddTrackToGroup(d, g.ID, id); err != nil {
			t.Fatalf("AddTrackToGroup(%s): %v", id, err)
		}
	}
	// Idempotent re-add.
	if err := AddTrackToGroup(d, g.ID, "v1"); err != nil {
		t.Fatalf("re-add v1: %v", err)
	}

	detail, err := GetVariantGroupDetail(d, g.ID)
	if err != nil {
		t.Fatalf("GetVariantGroupDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("GetVariantGroupDetail nil")
	}
	if detail.Group.TrackCount != 3 || len(detail.Tracks) != 3 {
		t.Errorf("group tracks = %d/%d, want 3/3", detail.Group.TrackCount, len(detail.Tracks))
	}

	// Best track must be a member.
	if err := SetBestTrack(d, g.ID, "unknown"); err == nil {
		t.Error("set best to non-member should error")
	}
	if err := SetBestTrack(d, g.ID, "v2"); err != nil {
		t.Fatalf("SetBestTrack: %v", err)
	}
	detail, _ = GetVariantGroupDetail(d, g.ID)
	if detail.Group.BestTrackID != "v2" {
		t.Errorf("best = %q, want v2", detail.Group.BestTrackID)
	}

	// List groups shows count.
	groups, err := ListVariantGroups(d)
	if err != nil {
		t.Fatalf("ListVariantGroups: %v", err)
	}
	if len(groups) != 1 || groups[0].TrackCount != 3 || groups[0].BestTrackID != "v2" {
		t.Errorf("ListVariantGroups = %+v", groups)
	}

	// Removing the best track clears the selection.
	if err := RemoveTrackFromGroup(d, g.ID, "v2"); err != nil {
		t.Fatalf("RemoveTrackFromGroup: %v", err)
	}
	detail, _ = GetVariantGroupDetail(d, g.ID)
	if detail.Group.BestTrackID != "" {
		t.Errorf("best should be cleared, got %q", detail.Group.BestTrackID)
	}

	// Update + delete.
	if err := UpdateVariantGroup(d, g.ID, "Nights v2", "notes"); err != nil {
		t.Fatalf("UpdateVariantGroup: %v", err)
	}
	if err := DeleteVariantGroup(d, g.ID); err != nil {
		t.Fatalf("DeleteVariantGroup: %v", err)
	}
	detail, _ = GetVariantGroupDetail(d, g.ID)
	if detail != nil {
		t.Error("group should be gone")
	}
}

func TestSuggestVariantGroups(t *testing.T) {
	d := newTestDB(t)
	defer d.Close()

	mustTrack(t, d, "s1", "Dawn", true)
	mustTrack(t, d, "s2", "Dawn", true)
	mustTrack(t, d, "s3", "Dawn", false)
	mustTrack(t, d, "u1", "Twilight", true)
	mustTrack(t, d, "u2", "Sunset (live)", true)

	suggestions, err := SuggestVariantGroups(d)
	if err != nil {
		t.Fatalf("SuggestVariantGroups: %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("suggestions = %+v, want only 'Dawn'", suggestions)
	}
	s := suggestions[0]
	if s.Title != "Dawn" {
		t.Errorf("suggestion title = %q, want Dawn", s.Title)
	}
	if len(s.TrackIDs) != 3 {
		t.Errorf("Dawn track ids = %v, want 3", s.TrackIDs)
	}
	if s.AllDownloaded {
		t.Error("AllDownloaded should be false (s3 not downloaded)")
	}

	// Download s3 -> AllDownloaded turns true.
	if err := UpsertTrack(d, &models.Track{ID: "s3", Title: "Dawn", IsDownloaded: true}); err != nil {
		t.Fatalf("update s3: %v", err)
	}
	suggestions, _ = SuggestVariantGroups(d)
	if len(suggestions) != 1 || !suggestions[0].AllDownloaded {
		t.Errorf("AllDownloaded after update = %+v", suggestions)
	}
}
