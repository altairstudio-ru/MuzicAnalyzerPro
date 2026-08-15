package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenameTrackFilesSameDir(t *testing.T) {
	dir := t.TempDir()
	oldAudio := filepath.Join(dir, "Old Artist — Old Title [id].mp3")
	newAudio := filepath.Join(dir, "New Artist — New Title [id].mp3")

	if err := os.WriteFile(oldAudio, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTxt := filepath.Join(dir, "Old Artist — Old Title [id].txt")
	if err := os.WriteFile(oldTxt, []byte("lyrics"), 0o644); err != nil {
		t.Fatal(err)
	}

	lp, err := renameTrackFiles(oldAudio, newAudio)
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := os.Stat(oldAudio); !os.IsNotExist(err) {
		t.Errorf("old audio still exists")
	}
	if _, err := os.Stat(newAudio); err != nil {
		t.Errorf("new audio missing: %v", err)
	}
	if _, err := os.Stat(oldTxt); !os.IsNotExist(err) {
		t.Errorf("old lyrics still exists")
	}
	if lp != filepath.Join(dir, "New Artist — New Title [id].txt") {
		t.Errorf("lyrics path = %q", lp)
	}
	if _, err := os.Stat(lp); err != nil {
		t.Errorf("new lyrics missing: %v", err)
	}
}

func TestRenameTrackFilesMoveDir(t *testing.T) {
	base := t.TempDir()
	oldDir := filepath.Join(base, "ws-old")
	newDir := filepath.Join(base, "ws-new")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldAudio := filepath.Join(oldDir, "Title [id].mp3")
	newAudio := filepath.Join(newDir, "Title [id].mp3")

	if err := os.WriteFile(oldAudio, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTxt := filepath.Join(oldDir, "Title [id].txt")
	if err := os.WriteFile(oldTxt, []byte("lyrics"), 0o644); err != nil {
		t.Fatal(err)
	}

	lp, err := renameTrackFiles(oldAudio, newAudio)
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := os.Stat(oldAudio); !os.IsNotExist(err) {
		t.Errorf("old audio still exists")
	}
	if _, err := os.Stat(newAudio); err != nil {
		t.Errorf("new audio missing: %v", err)
	}
	if _, err := os.Stat(lp); err != nil {
		t.Errorf("new lyrics missing: %v", err)
	}
}

func TestRenameTrackFilesNoop(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "Title [id].mp3")
	if err := os.WriteFile(audio, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}

	lp, err := renameTrackFiles(audio, audio)
	if err != nil {
		t.Fatalf("noop rename error: %v", err)
	}
	if lp != "" {
		t.Errorf("expected empty lyrics path, got %q", lp)
	}
	if _, err := os.Stat(audio); err != nil {
		t.Errorf("audio missing after noop: %v", err)
	}
}

func TestRenameTrackFilesMissingTxt(t *testing.T) {
	dir := t.TempDir()
	oldAudio := filepath.Join(dir, "Old [id].mp3")
	newAudio := filepath.Join(dir, "New [id].mp3")
	if err := os.WriteFile(oldAudio, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}

	lp, err := renameTrackFiles(oldAudio, newAudio)
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if lp != "" {
		t.Errorf("expected empty lyrics path (no txt), got %q", lp)
	}
	if _, err := os.Stat(newAudio); err != nil {
		t.Errorf("new audio missing: %v", err)
	}
}

func TestRenameTrackFilesMissingSource(t *testing.T) {
	dir := t.TempDir()
	oldAudio := filepath.Join(dir, "Missing [id].mp3")
	newAudio := filepath.Join(dir, "New [id].mp3")

	if _, err := renameTrackFiles(oldAudio, newAudio); err == nil {
		t.Errorf("expected error for missing source")
	}
	if _, err := os.Stat(newAudio); !os.IsNotExist(err) {
		t.Errorf("new audio unexpectedly created")
	}
}

func TestRenameTrackFilesLyricsMoveFailIsNonFatal(t *testing.T) {
	dir := t.TempDir()
	oldAudio := filepath.Join(dir, "Old [id].mp3")
	newAudio := filepath.Join(dir, "New [id].mp3")
	newLyrics := filepath.Join(dir, "New [id].txt")

	if err := os.WriteFile(oldAudio, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTxt := filepath.Join(dir, "Old [id].txt")
	if err := os.WriteFile(oldTxt, []byte("lyrics"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Make the new lyrics path non-movable: a non-empty directory blocks both
	// os.Rename and the copy fallback, deterministically failing moveOrCopy.
	if err := os.MkdirAll(newLyrics, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newLyrics, "blocker"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	lp, err := renameTrackFiles(oldAudio, newAudio)
	if err != nil {
		t.Errorf("lyrics move failure must not fail the rename: %v", err)
	}
	if _, err := os.Stat(newAudio); err != nil {
		t.Errorf("new audio missing: %v", err)
	}
	if _, err := os.Stat(oldAudio); !os.IsNotExist(err) {
		t.Errorf("old audio still exists")
	}
	if lp != "" {
		t.Errorf("expected empty lyrics path (move failed), got %q", lp)
	}
	// The old lyrics stay in place so LyricsPath stays valid until saveLyrics
	// rewrites them next to the new audio.
	if _, err := os.Stat(oldTxt); err != nil {
		t.Errorf("old lyrics should remain: %v", err)
	}
}
