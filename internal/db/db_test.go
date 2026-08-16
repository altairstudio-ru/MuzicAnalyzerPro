package db

import (
	"database/sql"
	"testing"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

func TestInit(t *testing.T) {
	_, err := Init(":memory:")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
}

func TestUpsertAndGetTrack(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	track := &models.Track{
		ID:        "test-123",
		Title:     "Test Song",
		Artist:    "Test Artist",
		Prompt:    "epic orchestral rock",
		Lyrics:    "Test lyrics here",
		Tags:      []string{"epic", "orchestral"},
		Workspace: "My Workspace",
		Duration:  180,
		CreatedAt: "2024-01-01T00:00:00Z",
	}

	if err := UpsertTrack(db, track); err != nil {
		t.Fatalf("UpsertTrack() error: %v", err)
	}

	got, err := GetTrack(db, "test-123")
	if err != nil {
		t.Fatalf("GetTrack() error: %v", err)
	}
	if got == nil {
		t.Fatal("GetTrack() returned nil")
	}
	if got.Title != track.Title {
		t.Errorf("Title = %q, want %q", got.Title, track.Title)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "epic" {
		t.Errorf("Tags = %v, want [epic orchestral]", got.Tags)
	}
}

func TestListTracks(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	tracks := []*models.Track{
		{ID: "1", Title: "Song A", Tags: []string{"rock"}, Workspace: "W1"},
		{ID: "2", Title: "Song B", Tags: []string{"jazz"}, Workspace: "W2"},
		{ID: "3", Title: "Song C", Tags: []string{"rock", "epic"}, Workspace: "W1"},
	}

	for _, tr := range tracks {
		if err := UpsertTrack(db, tr); err != nil {
			t.Fatalf("UpsertTrack() error: %v", err)
		}
	}

	// Test workspace filter
	filter := models.TrackFilter{Workspace: "W1"}
	result, err := ListTracks(db, filter)
	if err != nil {
		t.Fatalf("ListTracks() error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("W1: got %d tracks, want 2", len(result))
	}

	// Test tag filter
	filter = models.TrackFilter{Tag: "jazz"}
	result, err = ListTracks(db, filter)
	if err != nil {
		t.Fatalf("ListTracks() error: %v", err)
	}
	if len(result) != 1 || result[0].ID != "2" {
		t.Errorf("jazz tag: got %v, want [2]", ids(result))
	}
}

func TestTrackSunoMetricsRoundTrip(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	track := &models.Track{
		ID:          "metrics-1",
		Title:       "Liked Song",
		UpvoteCount: 42,
		PlayCount:   1000,
		IsLiked:     true,
		TrackType:   "cover",
		ModelName:   "chirp-v3",
	}
	if err := UpsertTrack(db, track); err != nil {
		t.Fatalf("UpsertTrack: %v", err)
	}

	got, err := GetTrack(db, "metrics-1")
	if err != nil || got == nil {
		t.Fatalf("GetTrack: %v %v", got, err)
	}
	if got.UpvoteCount != 42 || got.PlayCount != 1000 || !got.IsLiked {
		t.Errorf("metrics = upvote=%d play=%d liked=%v", got.UpvoteCount, got.PlayCount, got.IsLiked)
	}
	if got.TrackType != "cover" || got.ModelName != "chirp-v3" {
		t.Errorf("type/model = %q %q", got.TrackType, got.ModelName)
	}

	list, err := ListTracks(db, models.TrackFilter{TrackType: "cover"})
	if err != nil {
		t.Fatalf("ListTracks: %v", err)
	}
	if len(list) != 1 || list[0].ID != "metrics-1" {
		t.Errorf("track_type filter: got %v", ids(list))
	}
}

func TestWorkspaceCRUD(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	w := &models.Workspace{Name: "Test WS", TrackCount: 5, SyncedAt: "2024-01-01"}
	if err := UpsertWorkspace(db, w); err != nil {
		t.Fatalf("UpsertWorkspace() error: %v", err)
	}

	ws, err := ListWorkspaces(db)
	if err != nil {
		t.Fatalf("ListWorkspaces() error: %v", err)
	}
	if len(ws) != 1 || ws[0].Name != "Test WS" {
		t.Errorf("got %v, want [Test WS]", ws)
	}
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Init(":memory:")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	return db
}

func ids(tracks []models.Track) []string {
	var out []string
	for _, t := range tracks {
		out = append(out, t.ID)
	}
	return out
}
