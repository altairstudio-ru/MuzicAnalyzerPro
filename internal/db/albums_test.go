package db

import (
	"database/sql"
	"testing"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

func mustTrack(t *testing.T, db *sql.DB, id, title string, downloaded bool) {
	t.Helper()
	err := UpsertTrack(db, &models.Track{ID: id, Title: title, IsDownloaded: downloaded})
	if err != nil {
		t.Fatalf("UpsertTrack(%s): %v", id, err)
	}
}

func TestAlbumCRUD(t *testing.T) {
	d := newTestDB(t)
	defer d.Close()

	a := &models.Album{Title: "Neon Nights", Kind: "album", Notes: "debut"}
	if err := CreateAlbum(d, a); err != nil {
		t.Fatalf("CreateAlbum: %v", err)
	}
	if a.ID == "" {
		t.Fatal("CreateAlbum did not generate ID")
	}

	mustTrack(t, d, "trk-1", "Song One", true)
	mustTrack(t, d, "trk-2", "Song Two", true)
	mustTrack(t, d, "trk-3", "Song Three", false)

	// Appended positions: 1, 2, 3.
	if err := AddTrackToAlbum(d, a.ID, "trk-2", 0, "lead single"); err != nil {
		t.Fatalf("AddTrackToAlbum 1: %v", err)
	}
	if err := AddTrackToAlbum(d, a.ID, "trk-1", 0, ""); err != nil {
		t.Fatalf("AddTrackToAlbum 2: %v", err)
	}
	if err := AddTrackToAlbum(d, a.ID, "trk-3", 0, ""); err != nil {
		t.Fatalf("AddTrackToAlbum 3: %v", err)
	}

	// Move trk-1 to the end (position 3), shifting trk-3 up to 2.
	if err := SetAlbumTrackPosition(d, a.ID, "trk-1", 3); err != nil {
		t.Fatalf("SetAlbumTrackPosition: %v", err)
	}
	if err := SetAlbumTrackPosition(d, a.ID, "trk-3", 2); err != nil {
		t.Fatalf("SetAlbumTrackPosition: %v", err)
	}

	out, err := GetAlbumWithTracks(d, a.ID)
	if err != nil {
		t.Fatalf("GetAlbumWithTracks: %v", err)
	}
	if out == nil {
		t.Fatal("GetAlbumWithTracks returned nil")
	}
	if out.Album.Title != "Neon Nights" || out.Album.TrackCount != 3 {
		t.Errorf("album meta = %+v", out.Album)
	}
	if out.TotalDuration != 0 {
		t.Errorf("TotalDuration = %d, want 0 (no durations set)", out.TotalDuration)
	}
	wantOrder := []string{"trk-2", "trk-3", "trk-1"}
	gotOrder := make([]string, 0, 3)
	for _, it := range out.Tracks {
		gotOrder = append(gotOrder, it.Track.ID)
	}
	if len(gotOrder) != 3 || gotOrder[0] != wantOrder[0] || gotOrder[1] != wantOrder[1] || gotOrder[2] != wantOrder[2] {
		t.Errorf("order = %v, want %v", gotOrder, wantOrder)
	}

	// Reorder to explicit sequence.
	if err := ReorderAlbumTracks(d, a.ID, []string{"trk-1", "trk-2", "trk-3"}); err != nil {
		t.Fatalf("ReorderAlbumTracks: %v", err)
	}
	out, _ = GetAlbumWithTracks(d, a.ID)
	if out.Tracks[0].Track.ID != "trk-1" || out.Tracks[2].Track.ID != "trk-3" {
		t.Errorf("after reorder order = %v", orderIds(out.Tracks))
	}

	// Remove a track.
	if err := RemoveTrackFromAlbum(d, a.ID, "trk-2"); err != nil {
		t.Fatalf("RemoveTrackFromAlbum: %v", err)
	}
	out, _ = GetAlbumWithTracks(d, a.ID)
	if out.Album.TrackCount != 2 {
		t.Errorf("after remove TrackCount = %d, want 2", out.Album.TrackCount)
	}

	// List albums shows the count.
	all, err := ListAlbums(d)
	if err != nil {
		t.Fatalf("ListAlbums: %v", err)
	}
	if len(all) != 1 || all[0].TrackCount != 2 {
		t.Errorf("ListAlbums = %+v", all)
	}

	// Update album.
	if err := UpdateAlbum(d, a.ID, "Renamed", "compilation", "notes"); err != nil {
		t.Fatalf("UpdateAlbum: %v", err)
	}
	out, _ = GetAlbumWithTracks(d, a.ID)
	if out.Album.Title != "Renamed" || out.Album.Kind != "compilation" {
		t.Errorf("updated album = %+v", out.Album)
	}

	// Delete album cascades track entries.
	if err := DeleteAlbum(d, a.ID); err != nil {
		t.Fatalf("DeleteAlbum: %v", err)
	}
	out, _ = GetAlbumWithTracks(d, a.ID)
	if out != nil {
		t.Error("album should be gone after delete")
	}
	var leftover int
	if err := d.QueryRow("SELECT COUNT(*) FROM album_tracks WHERE album_id = ?", a.ID).Scan(&leftover); err != nil {
		t.Fatalf("count leftover: %v", err)
	}
	if leftover != 0 {
		t.Errorf("leftover album_tracks = %d, want 0", leftover)
	}
}

func TestListTracksByAlbumUsesFilter(t *testing.T) {
	d := newTestDB(t)
	defer d.Close()

	a := &models.Album{Title: "A"}
	if err := CreateAlbum(d, a); err != nil {
		t.Fatalf("CreateAlbum: %v", err)
	}
	mustTrack(t, d, "t1", "One", true)
	mustTrack(t, d, "t2", "Two", true)
	if err := AddTrackToAlbum(d, a.ID, "t1", 0, ""); err != nil {
		t.Fatalf("AddTrackToAlbum: %v", err)
	}
	if err := AddTrackToAlbum(d, a.ID, "t2", 0, ""); err != nil {
		t.Fatalf("AddTrackToAlbum: %v", err)
	}

	byAlbum, err := ListTracks(d, models.TrackFilter{AlbumID: a.ID})
	if err != nil {
		t.Fatalf("ListTracks album filter: %v", err)
	}
	if len(byAlbum) != 2 {
		t.Errorf("album filter got %d, want 2", len(byAlbum))
	}
	search, err := ListTracks(d, models.TrackFilter{AlbumID: a.ID, Search: "Two"})
	if err != nil {
		t.Fatalf("ListTracks album+search: %v", err)
	}
	if len(search) != 1 || search[0].ID != "t2" {
		t.Errorf("album+search = %v, want [t2]", ids(search))
	}
}

func TestTrackNotInAlbumError(t *testing.T) {
	d := newTestDB(t)
	defer d.Close()

	a := &models.Album{Title: "A"}
	_ = CreateAlbum(d, a)
	mustTrack(t, d, "t1", "One", false)

	if err := RemoveTrackFromAlbum(d, a.ID, "t1"); err == nil {
		t.Error("removing a track that was never added should error")
	}
	if err := AddTrackToAlbum(d, "missing-album", "t1", 0, ""); err == nil {
		t.Error("adding to a missing album should error (FK)")
	}
}

func orderIds(items []models.AlbumTrackItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Track.ID)
	}
	return out
}
