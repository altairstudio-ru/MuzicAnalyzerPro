package library

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/db"
	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"

	"gopkg.in/yaml.v3"
)

// ExportOptions controls an Obsidian vault export run.
type ExportOptions struct {
	// Vault is the target vault directory (e.g. ~/myMusic/Suno).
	// Empty disables the export.
	Vault string
	// Overwrite rewrites every note even when identical (full regen).
	Overwrite bool
}

// ExportStats reports how many notes were written / skipped.
type ExportStats struct {
	NotesWritten int
	NotesSkipped int
	CorpusTracks int
	Errors       int
}

// exportManifest maps note filename -> content hash so unchanged notes are
// left untouched and only new/modified tracks are re-exported.
type exportManifest map[string]string

const exportManifestName = ".export-manifest.json"

// noteFrontmatter is the YAML header written at the top of each note.
type noteFrontmatter struct {
	ID        string   `yaml:"id"`
	Title     string   `yaml:"title"`
	Artist    string   `yaml:"artist"`
	Workspace string   `yaml:"workspace"`
	Tags      []string `yaml:"tags"`
	Prompt    string   `yaml:"prompt"`
	Duration  int      `yaml:"duration"`
}

// ExportNotes writes each track as a markdown note under Vault/tracks and
// regenerates the training corpus under Vault/corpus. Only notes whose
// content changed are rewritten unless Overwrite is set. Returns a summary of
// the export run.
func (m *Manager) ExportNotes(opts ExportOptions) (*ExportStats, error) {
	if opts.Vault == "" {
		return &ExportStats{}, nil
	}
	vault := expandPath(opts.Vault)

	tracks, err := db.GetAllTracks(m.DB)
	if err != nil {
		return nil, err
	}

	tracksDir := filepath.Join(vault, "tracks")
	corpusDir := filepath.Join(vault, "corpus")
	if err := ensureVault(vault); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(tracksDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(corpusDir, 0o755); err != nil {
		return nil, err
	}

	stats := &ExportStats{}
	manifest := loadManifest(filepath.Join(vault, exportManifestName))
	next := exportManifest{}

	workspaceLyrics := make(map[string][]string)
	fullLyrics := []string{}

	for _, t := range tracks {
		lyrics := t.Lyrics
		if strings.TrimSpace(lyrics) == "" {
			if t.LyricsPath != "" {
				if data, err := os.ReadFile(t.LyricsPath); err == nil {
					lyrics = string(data)
				}
			}
		}

		content := renderNote(t, lyrics)
		hash := hashContent(content)

		name := noteFilename(t)
		if !opts.Overwrite && manifest[name] == hash {
			next[name] = hash
			stats.NotesSkipped++
		} else {
			notePath := filepath.Join(tracksDir, name)
			if err := os.WriteFile(notePath, []byte(content), 0o644); err != nil {
				stats.Errors++
			} else {
				next[name] = hash
				stats.NotesWritten++
			}
		}

		if strings.TrimSpace(lyrics) == "" {
			continue
		}
		header := "=== " + t.Title + " [" + t.ID + "] ==="
		fullLyrics = append(fullLyrics, header+"\n\n"+strings.TrimSpace(lyrics))

		ws := t.Workspace
		if ws == "" {
			ws = "Unknown"
		}
		workspaceLyrics[ws] = append(workspaceLyrics[ws], header+"\n\n"+strings.TrimSpace(lyrics))
		stats.CorpusTracks++
	}

	keep := make(map[string]bool, len(tracks))
	for _, t := range tracks {
		keep[noteFilename(t)] = true
	}
	if err := cleanupNotes(tracksDir, keep); err != nil {
		return stats, err
	}

	if err := writeCorpus(corpusDir, fullLyrics, workspaceLyrics); err != nil {
		return stats, err
	}

	if err := saveManifest(filepath.Join(vault, exportManifestName), next); err != nil {
		return stats, err
	}

	return stats, nil
}

// ensureVault creates the vault directory and, on first run, a bare .obsidian
// folder so Obsidian and the MCP vault server recognize it. If any parent
// directory is already an Obsidian vault (has .obsidian), a nested one is not
// created so a sub-folder export stays part of the existing vault.
func ensureVault(vault string) error {
	if err := os.MkdirAll(vault, 0o755); err != nil {
		return err
	}
	for dir := filepath.Dir(vault); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		obsidianDir := filepath.Join(dir, ".obsidian")
		if fi, err := os.Stat(obsidianDir); err == nil && fi.IsDir() {
			return nil
		}
	}
	obsidianDir := filepath.Join(vault, ".obsidian")
	if fi, err := os.Stat(obsidianDir); err == nil && fi.IsDir() {
		return nil
	}
	return os.MkdirAll(obsidianDir, 0o755)
}

// noteFilename builds an Obsidian note name from a track, mirroring the audio
// file naming: "Artist — Title [id].md". Falls back to "Title [id].md" when
// the artist is empty, to "Artist [id].md" when the title is empty, and to
// "[id].md" when both are. The ID is always present and keeps names unique
// even for identical titles.
func noteFilename(t models.Track) string {
	artist := sanitizeFilename(t.Artist)
	title := sanitizeFilename(t.Title)

	switch {
	case artist != "" && title != "":
		return artist + " — " + title + " [" + t.ID + "].md"
	case title != "":
		return title + " [" + t.ID + "].md"
	case artist != "":
		return artist + " [" + t.ID + "].md"
	default:
		return "[" + t.ID + "].md"
	}
}

// noteNameRe matches both note name formats we write: "<uuid>.md" (legacy)
// and "<anything> [<uuid>].md" (current), so cleanup never removes notes the
// user created by hand.
var noteNameRe = regexp.MustCompile(`^(?:.* \[)?[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\]?\.md$`)

// cleanupNotes removes stale note files (renamed/deleted tracks) from the
// tracks directory. Only files matching our generated naming are touched;
// anything else in the directory is left alone.
func cleanupNotes(tracksDir string, keep map[string]bool) error {
	entries, err := os.ReadDir(tracksDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !noteNameRe.MatchString(e.Name()) {
			continue
		}
		if !keep[e.Name()] {
			if err := os.Remove(filepath.Join(tracksDir, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// renderNote builds a complete markdown note for a track. When the track has
// downloaded audio, an Obsidian embed (![[..mp3]]) is added under the
// frontmatter so the audio can be played right from the note. The audio must
// live inside the vault (or be reachable through a symlink placed in it, e.g.
// <vault>/audio -> the audio base path); the filename is unique thanks to the
// [uuid] suffix, so Obsidian resolves the embed globally.
func renderNote(t models.Track, lyrics string) string {
	fm := noteFrontmatter{
		ID:        t.ID,
		Title:     t.Title,
		Artist:    t.Artist,
		Workspace: t.Workspace,
		Tags:      t.Tags,
		Prompt:    t.Prompt,
		Duration:  t.Duration,
	}
	header, _ := yaml.Marshal(fm)

	var b strings.Builder
	b.WriteString("---\n")
	b.Write(header)
	b.WriteString("---\n\n")
	if t.AudioPath != "" {
		b.WriteString("![[")
		b.WriteString(filepath.Base(t.AudioPath))
		b.WriteString("]]\n\n")
	}
	if strings.TrimSpace(lyrics) != "" {
		b.WriteString(strings.TrimSpace(lyrics))
		b.WriteString("\n")
	}
	return b.String()
}

// writeCorpus regenerates the whole corpus on every run: corpus/all.txt and
// corpus/by-workspace/<ws>.txt for each workspace, removing stale files from
// earlier runs so renamed/deleted workspaces do not linger.
func writeCorpus(corpusDir string, full []string, byWorkspace map[string][]string) error {
	byWsDir := filepath.Join(corpusDir, "by-workspace")
	if err := os.MkdirAll(byWsDir, 0o755); err != nil {
		return err
	}

	keep := map[string]bool{}
	if len(full) > 0 {
		allPath := filepath.Join(corpusDir, "all.txt")
		if err := os.WriteFile(allPath, []byte(strings.Join(full, "\n\n")+"\n"), 0o644); err != nil {
			return err
		}
		keep["all.txt"] = true
	}

	workspaces := make([]string, 0, len(byWorkspace))
	for ws := range byWorkspace {
		workspaces = append(workspaces, ws)
	}
	sort.Strings(workspaces)

	for _, ws := range workspaces {
		lines := byWorkspace[ws]
		if len(lines) == 0 {
			continue
		}
		name := sanitizeDirName(ws)
		if name == "" {
			name = "Unknown"
		}
		path := filepath.Join(byWsDir, name+".txt")
		keep[path] = true
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n\n")+"\n"), 0o644); err != nil {
			return err
		}
	}

	for _, dir := range []string{corpusDir, byWsDir} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".txt" {
				continue
			}
			p := filepath.Join(dir, e.Name())
			if !keep[p] {
				if err := os.Remove(p); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// hashContent returns the SHA-256 hex digest of a note's rendered content.
func hashContent(content string) string {
	h := sha256.New()
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}

// loadManifest reads a previously written export manifest, tolerating absence.
func loadManifest(path string) exportManifest {
	data, err := os.ReadFile(path)
	if err != nil {
		return exportManifest{}
	}
	m := exportManifest{}
	if err := json.Unmarshal(data, &m); err != nil {
		return exportManifest{}
	}
	return m
}

// saveManifest persists the export manifest as JSON.
func saveManifest(path string, m exportManifest) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
