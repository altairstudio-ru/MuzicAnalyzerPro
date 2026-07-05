package library

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/db"
	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/suno"
	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

// Manager orchestrates sync operations between Suno API and local storage.
type Manager struct {
	Config *Config
	Suno   *suno.Client
	DB     *sql.DB
}

// NewManager creates a new library manager using the given config.
// It initializes the Suno client and database.
func NewManager(cfg *Config) (*Manager, error) {
	if cfg.Suno.AuthToken == "" {
		return nil, fmt.Errorf("auth token required — run 'suno-archiver auth <token>' first")
	}

	sunoClient := suno.NewClient(cfg.Suno.AuthToken)

	database, err := db.Init(cfg.DBPath())
	if err != nil {
		return nil, fmt.Errorf("init database: %w", err)
	}

	return &Manager{
		Config: cfg,
		Suno:   sunoClient,
		DB:     database,
	}, nil
}

// Close cleans up resources.
func (m *Manager) Close() error {
	if m.DB != nil {
		return m.DB.Close()
	}
	return nil
}

// Sync performs a full sync: fetch all tracks from Suno, update DB, download audio.
func (m *Manager) Sync() (*models.SyncStats, error) {
	stats := &models.SyncStats{}

	// 1. Fetch all tracks from Suno
	apiTracks, err := m.Suno.FetchAllTracks()
	if err != nil {
		return nil, fmt.Errorf("fetch tracks: %w", err)
	}
	stats.TotalTracks = len(apiTracks)

	// 2. Process each track
	workspaceSet := make(map[string]bool)
	for _, track := range apiTracks {
		isNew := false

		// Check if track already exists
		existing, err := db.GetTrack(m.DB, track.ID)
		if err != nil {
			stats.Errors++
			continue
		}
		if existing == nil {
			isNew = true
		}

		// Track workspace
		if track.Workspace != "" {
			workspaceSet[track.Workspace] = true
		}

		// Set audio path
		audioPath := m.audioPath(track)

		// Try to download if not already downloaded
		if existing == nil || !existing.IsDownloaded {
			err := m.downloadTrack(track, audioPath)
			if err != nil {
				stats.Errors++
				// Still upsert the metadata even if download fails
			} else {
				stats.Downloaded++
				track.IsDownloaded = true
				track.AudioPath = audioPath
				// Get file info
				if fi, err := os.Stat(audioPath); err == nil {
					track.FileSize = fi.Size()
				}
				// Compute hash
				if hash, err := fileHash(audioPath); err == nil {
					track.AudioHash = hash
				}
			}
		} else {
			track.IsDownloaded = true
			track.AudioPath = existing.AudioPath
			track.AudioHash = existing.AudioHash
			track.FileSize = existing.FileSize
		}

		// Upsert to database
		if err := db.UpsertTrack(m.DB, &track); err != nil {
			stats.Errors++
			continue
		}

		if isNew {
			stats.NewTracks++
		} else {
			stats.UpdatedTracks++
		}
	}

	// 3. Update workspace entries
	for ws := range workspaceSet {
		if err := db.UpsertWorkspace(m.DB, &models.Workspace{
			Name: ws,
		}); err != nil {
			// Non-fatal
			continue
		}
		if err := db.UpdateWorkspaceTrackCount(m.DB, ws); err != nil {
			continue
		}
	}

	return stats, nil
}

// downloadTrack downloads a track's audio file.
func (m *Manager) downloadTrack(track models.Track, dest string) error {
	// Create directory
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return m.Suno.DownloadAudio(track.ID, dest)
}

// audioPath returns the expected local path for a track's audio file.
func (m *Manager) audioPath(track models.Track) string {
	ws := track.Workspace
	if ws == "" {
		ws = "Unknown"
	}
	dir := m.Config.WorkspaceAudioDir(ws)

	// Sanitize filename
	title := sanitizeFilename(track.Title)
	if title == "" {
		title = track.ID
	}

	artist := sanitizeFilename(track.Artist)
	var filename string
	if artist != "" {
		filename = fmt.Sprintf("%s — %s [%s].mp3", artist, title, track.ID)
	} else {
		filename = fmt.Sprintf("%s [%s].mp3", title, track.ID)
	}

	return filepath.Join(dir, filename)
}

// sanitizeFilename removes characters unsafe for filenames.
func sanitizeFilename(name string) string {
	if name == "" {
		return name
	}
	result := make([]byte, 0, len(name))
	for _, c := range []byte(name) {
		if c == '/' || c == '\\' || c == '\x00' {
			result = append(result, '_')
		} else {
			result = append(result, c)
		}
	}
	return string(result)
}

// fileHash computes the SHA256 hash of a file.
func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
