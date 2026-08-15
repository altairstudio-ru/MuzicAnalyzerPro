package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// renameTrackFiles moves a downloaded track's audio file (and its lyrics .txt
// next to it, when present) to a new path. The destination directory is created
// as needed. It returns the new lyrics path (empty when there was none).
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
		if err := os.Rename(oldLyrics, newLyrics); err != nil {
			return "", fmt.Errorf("rename lyrics: %w", err)
		}
		return newLyrics, nil
	}
	return "", nil
}
