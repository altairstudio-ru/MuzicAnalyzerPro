package library

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// renameTrackFiles moves a downloaded track's audio file (and its lyrics .txt
// next to it, when present) to a new path. The destination directory is created
// as needed. It returns the new lyrics path (empty when there was none).
//
// The audio move is authoritative: on failure nothing has moved and the error
// is returned so the caller keeps the old path. The lyrics file is secondary —
// on failure its path is left unchanged (the txt stays at the old location and
// saveLyrics rewrites it next to the new audio on the next sync), so a lyrics
// hiccup never leaves the audio pointer stale.
func renameTrackFiles(oldAudio, newAudio string) (string, error) {
	if oldAudio == "" || newAudio == "" || oldAudio == newAudio {
		return "", nil
	}

	if err := os.MkdirAll(filepath.Dir(newAudio), 0o755); err != nil {
		return "", fmt.Errorf("create destination directory: %w", err)
	}

	if err := os.Rename(oldAudio, newAudio); err != nil {
		return "", fmt.Errorf("rename audio: %w", err)
	}

	oldLyrics := strings.TrimSuffix(oldAudio, filepath.Ext(oldAudio)) + ".txt"
	newLyrics := strings.TrimSuffix(newAudio, filepath.Ext(newAudio)) + ".txt"
	if fi, err := os.Stat(oldLyrics); err == nil && !fi.IsDir() {
		if err := moveOrCopy(oldLyrics, newLyrics); err != nil {
			log.Printf("[rename] lyrics move %s -> %s: %v (audio already moved; lyrics retried by next sync)", oldLyrics, newLyrics, err)
			return "", nil
		}
		return newLyrics, nil
	}
	return "", nil
}

// moveOrCopy moves src to dst, falling back to copy+remove (e.g. when the
// paths are on different filesystems). The source is gone on success.
func moveOrCopy(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(dst)
		return err
	}
	return os.Remove(src)
}
