package library

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/db"
	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

const testID = "2a153ecb-b464-4892-a553-86f510d6a8ef"

func TestNoteFilename(t *testing.T) {
	cases := []struct {
		title  string
		artist string
		want   string
	}{
		{"Ангел мой", "Настя", "Настя — Ангел мой [" + testID + "].md"},
		{"Гудки", "", "Гудки [" + testID + "].md"},
		{"", "Настя", "Настя [" + testID + "].md"},
		{"", "", "[" + testID + "].md"},
		{"a:b/c", "d*e", "d_e — a_b_c [" + testID + "].md"},
	}
	for _, c := range cases {
		got := noteFilename(models.Track{ID: testID, Title: c.title, Artist: c.artist})
		if got != c.want {
			t.Errorf("noteFilename(%q,%q) = %q, want %q", c.title, c.artist, got, c.want)
		}
	}
}

func TestNoteFilenameUniqueForSameTitle(t *testing.T) {
	a := noteFilename(models.Track{ID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Title: "Same"})
	b := noteFilename(models.Track{ID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", Title: "Same"})
	if a == b {
		t.Errorf("expected distinct names, got %q", a)
	}
}

func TestCleanupNotes(t *testing.T) {
	dir := t.TempDir()
	genID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

	// Files that look like our generated notes.
	staleID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	stale := filepath.Join(dir, "Stale ["+staleID+"].md")
	if err := os.WriteFile(stale, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(dir, genID+".md")
	if err := os.WriteFile(legacy, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(dir, "Keep ["+genID+"].md")
	if err := os.WriteFile(keep, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	manual := filepath.Join(dir, "My Handwritten Note.md")
	if err := os.WriteFile(manual, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := cleanupNotes(dir, map[string]bool{filepath.Base(keep): true}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale note not removed")
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("legacy note not removed")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("kept note missing: %v", err)
	}
	if _, err := os.Stat(manual); err != nil {
		t.Errorf("manual note removed: %v", err)
	}
}

func TestCleanupNotesDoesNotMatchNonNote(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "Random folder note.md")
	if err := os.WriteFile(bad, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cleanupNotes(dir, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bad); err != nil {
		t.Errorf("non-note file removed: %v", err)
	}
}

// TestExportNotesNaming exercises the full ExportNotes path: it writes notes
// with the new "Artist — Title [id].md" naming and removes a legacy "<id>.md"
// note from a previous export.
func TestExportNotesNaming(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Init(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	track := models.Track{
		ID:           testID,
		Title:        "Гудки",
		Artist:       "Настя",
		Lyrics:       "текст песни",
		IsDownloaded: true,
	}
	if err := db.UpsertTrack(database, &track); err != nil {
		t.Fatal(err)
	}

	vault := t.TempDir()
	// Legacy note from an older export scheme that must be cleaned up.
	legacyPath := filepath.Join(vault, "tracks", testID+".md")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := &Manager{DB: database}
	stats, err := mgr.ExportNotes(ExportOptions{Vault: vault})
	if err != nil {
		t.Fatal(err)
	}

	wantName := "Настя — Гудки [" + testID + "].md"
	newPath := filepath.Join(vault, "tracks", wantName)
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("new note missing: %v", err)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Errorf("legacy note not cleaned up")
	}
	if stats.NotesWritten != 1 {
		t.Errorf("NotesWritten = %d, want 1", stats.NotesWritten)
	}
}
