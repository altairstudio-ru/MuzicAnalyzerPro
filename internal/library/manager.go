package library

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/db"
	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/suno"
	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

// Manager orchestrates sync operations between Suno API and local storage.
type Manager struct {
	Config *Config
	Suno   *suno.Client
	DB     *sql.DB
	status *SyncStatus
	// RenameOnSync is read at sync time; when true, downloaded audio/lyrics
	// files are moved to match the current title/workspace from the feed.
	RenameOnSync bool
	// cancel is closed when the user requests the running sync to stop.
	// A fresh channel is created at the start of every sync run.
	cancel chan struct{}
}

// SyncPhase describes the current stage of a sync run.
type SyncPhase string

const (
	PhaseFetch    SyncPhase = "fetch"    // paginating the Suno feed
	PhaseDownload SyncPhase = "download" // downloading track audio
	PhaseDone     SyncPhase = "done"     // finished (or failed)
)

// SyncStatus holds live progress of a sync run, safe for concurrent access
// so the web UI can poll it while the background sync goroutine updates it.
type SyncStatus struct {
	mu         sync.Mutex
	Running    bool
	Phase      SyncPhase
	Processed  int
	New        int
	Downloaded int
	Errors     int
	LastTrack  string
	StartedAt  time.Time
	FinishedAt time.Time
	ErrMsg     string
	Stopped    bool
}

func newSyncStatus() *SyncStatus {
	return &SyncStatus{}
}

// GetSyncStatus returns a snapshot of the current sync status.
func (m *Manager) GetSyncStatus() SyncStatus {
	m.status.mu.Lock()
	defer m.status.mu.Unlock()
	return SyncStatus{
		Running:    m.status.Running,
		Phase:      m.status.Phase,
		Processed:  m.status.Processed,
		New:        m.status.New,
		Downloaded: m.status.Downloaded,
		Errors:     m.status.Errors,
		LastTrack:  m.status.LastTrack,
		StartedAt:  m.status.StartedAt,
		FinishedAt: m.status.FinishedAt,
		ErrMsg:     m.status.ErrMsg,
		Stopped:    m.status.Stopped,
	}
}

// beginSync marks a sync run as started; returns false if one is running.
func (m *Manager) beginSync() bool {
	m.status.mu.Lock()
	defer m.status.mu.Unlock()
	if m.status.Running {
		return false
	}
	m.status.Running = true
	m.status.Phase = PhaseFetch
	m.status.Processed = 0
	m.status.New = 0
	m.status.Downloaded = 0
	m.status.Errors = 0
	m.status.LastTrack = ""
	m.status.StartedAt = time.Now()
	m.status.FinishedAt = time.Time{}
	m.status.ErrMsg = ""
	m.status.Stopped = false
	m.cancel = make(chan struct{})
	return true
}

// updateSync mutates the status with the given function applied under lock.
func (m *Manager) updateSync(fn func(s *SyncStatus)) {
	m.status.mu.Lock()
	defer m.status.mu.Unlock()
	fn(m.status)
}

// finishSync marks the sync run complete, preserving a previously set error
// message (e.g. one recorded before an early-return error path).
func (m *Manager) finishSync() {
	m.status.mu.Lock()
	defer m.status.mu.Unlock()
	m.status.Running = false
	m.status.Phase = PhaseDone
	m.status.FinishedAt = time.Now()
}

// StopSync requests a running sync to stop at the next safe point (end of the
// current track or page). It is safe to call when no sync is running.
func (m *Manager) StopSync() {
	m.status.mu.Lock()
	m.cancel = make(chan struct{})
	m.status.Stopped = true
	close(m.cancel)
	m.status.mu.Unlock()
}

// isCanceled reports whether the user requested the running sync to stop.
func (m *Manager) isCanceled() bool {
	m.status.mu.Lock()
	defer m.status.mu.Unlock()
	return m.cancel != nil && m.status.Stopped
}

// NewManager creates a new library manager using the given config.
// It initializes the Suno client and database.
func NewManager(cfg *Config) (*Manager, error) {
	token := effectiveAuthToken(cfg)
	if token == "" {
		return nil, fmt.Errorf("auth token required — run 'suno-archiver auth <token>' or send auth from the Chrome extension")
	}

	sunoClient := suno.NewClient(token)

	database, err := db.Init(cfg.DBPath())
	if err != nil {
		return nil, fmt.Errorf("init database: %w", err)
	}

	return &Manager{
		Config: cfg,
		Suno:   sunoClient,
		DB:     database,
		status: newSyncStatus(),
	}, nil
}

// Close cleans up resources.
func (m *Manager) Close() error {
	if m.DB != nil {
		return m.DB.Close()
	}
	return nil
}

// SetAuthToken updates the live Suno client's auth token and the in-memory
// config so subsequent API calls use the new session cookie without a restart.
func (m *Manager) SetAuthToken(token string) {
	m.Suno.SetAuthToken(token)
	m.Config.Suno.SessionCookie = token
}

// Sync performs a full sync: fetch all tracks from Suno, update DB, download audio.
func (m *Manager) Sync() (*models.SyncStats, error) {
	if !m.beginSync() {
		return nil, errors.New("sync already in progress")
	}
	defer m.finishSync()
	return m.syncOnce()
}

// TrySyncBackground atomically reserves the sync slot and runs the sync in a
// background goroutine. Reports progress via GetSyncStatus. Returns false if a
// sync is already in progress.
func (m *Manager) TrySyncBackground() bool {
	if !m.beginSync() {
		return false
	}
	go func() {
		defer m.finishSync()
		if _, err := m.syncOnce(); err != nil {
			log.Printf("Sync error: %v", err)
		}
	}()
	return true
}

// syncOnce runs a single sync pass. The caller must have reserved the slot via
// beginSync (or TrySyncBackground).
func (m *Manager) syncOnce() (*models.SyncStats, error) {
	stats := &models.SyncStats{}

	pageSize := 50
	cursor := ""
	workspaceSet := make(map[string]bool)

	for {
		if m.isCanceled() {
			st := stats
			m.updateSync(func(s *SyncStatus) {
				s.ErrMsg = "stopped by user"
			})
			return st, nil
		}
		resp, err := m.Suno.FetchTracks(cursor, pageSize)
		if err != nil {
			if suno.IsRateLimited(err) && stats.TotalTracks > 0 {
				m.updateSync(func(s *SyncStatus) {
					s.ErrMsg = fmt.Sprintf("rate limited at cursor %q (processed %d tracks)", cursor, stats.TotalTracks)
				})
				return stats, fmt.Errorf("rate limited at cursor %q (processed %d tracks)", cursor, stats.TotalTracks)
			}
			m.updateSync(func(s *SyncStatus) {
				s.ErrMsg = fmt.Sprintf("fetch cursor %q: %v", cursor, err)
			})
			return nil, fmt.Errorf("fetch cursor %q: %w", cursor, err)
		}
		m.updateSync(func(s *SyncStatus) {
			s.Phase = PhaseFetch
		})

		for _, track := range resp.Tracks {
			isNew := false

			m.updateSync(func(s *SyncStatus) {
				s.Phase = PhaseFetch
				s.LastTrack = track.Title
			})

			existing, err := db.GetTrack(m.DB, track.ID)
			if err != nil {
				stats.Errors++
				continue
			}
			if existing == nil {
				isNew = true
			}

			if track.Workspace != "" {
				workspaceSet[track.Workspace] = true
			}

			audioPath := m.audioPath(track)

			if existing == nil || !existing.IsDownloaded {
				m.updateSync(func(s *SyncStatus) {
					s.Phase = PhaseDownload
				})
				err := m.downloadTrack(track, audioPath)
				if err != nil {
					stats.Errors++
				} else {
					stats.Downloaded++
					track.IsDownloaded = true
					track.AudioPath = audioPath
					if fi, err := os.Stat(audioPath); err == nil {
						track.FileSize = fi.Size()
					}
					if hash, err := fileHash(audioPath); err == nil {
						track.AudioHash = hash
					}
				}
			} else {
				track.IsDownloaded = true
				track.AudioPath = existing.AudioPath
				track.AudioHash = existing.AudioHash
				track.FileSize = existing.FileSize
				// The v3 API no longer returns lyrics — keep what we
				// already extracted from the prompt.
				if track.Lyrics == "" {
					track.Lyrics = existing.Lyrics
					track.LyricsPath = existing.LyricsPath
				}

				// Rename/move the on-disk files when the title or workspace
				// changed in Suno. No-op when nothing moved.
				if m.RenameOnSync && existing.AudioPath != "" && audioPath != existing.AudioPath {
					newLp, err := renameTrackFiles(existing.AudioPath, audioPath)
					if err != nil {
						stats.Errors++
						log.Printf("[sync] rename %s (%s): %v", track.ID, track.Title, err)
					} else {
						track.AudioPath = audioPath
						if newLp != "" {
							track.LyricsPath = newLp
						}
					}
				}
			}

			if track.Lyrics != "" && track.AudioPath != "" {
				if lp, err := m.saveLyrics(track); err == nil {
					track.LyricsPath = lp
					stats.LyricsExported++
				}
			}

			if err := db.UpsertTrack(m.DB, &track); err != nil {
				stats.Errors++
				continue
			}

			stats.TotalTracks++

			if isNew {
				stats.NewTracks++
			} else {
				stats.UpdatedTracks++
			}

			m.updateSync(func(s *SyncStatus) {
				s.Processed = stats.TotalTracks
				s.New = stats.NewTracks
				s.Downloaded = stats.Downloaded
				s.Errors = stats.Errors
			})
		}

		if !resp.HasMore || len(resp.Tracks) == 0 {
			break
		}
		cursor = resp.Next
		time.Sleep(500 * time.Millisecond)
	}

	if m.isCanceled() {
		st := stats
		m.updateSync(func(s *SyncStatus) {
			s.ErrMsg = "stopped by user"
		})
		return st, nil
	}

	for ws := range workspaceSet {
		if err := db.UpsertWorkspace(m.DB, &models.Workspace{
			Name: ws,
		}); err != nil {
			continue
		}
		if err := db.UpdateWorkspaceTrackCount(m.DB, ws); err != nil {
			continue
		}
	}

	if err := m.exportAfterSync(); err != nil {
		m.updateSync(func(s *SyncStatus) {
			s.ErrMsg = fmt.Sprintf("export notes: %v", err)
		})
		log.Printf("Export notes: %v", err)
	}

	return stats, nil
}

// exportAfterSync reflects new/changed tracks into the Obsidian vault when an
// export_vault is configured. It rewrites only notes whose content changed.
func (m *Manager) exportAfterSync() error {
	vault := m.Config.Suno.ExportVault
	if vault == "" {
		return nil
	}
	_, err := m.ExportNotes(ExportOptions{Vault: vault})
	return err
}

// ImportFromDisk scans an audio directory for *.mp3 files and registers any
// tracks not present in the database. Metadata is enriched from the Suno feed
// where the track ID exists; otherwise the filename is used as the title.
// Files are marked as downloaded with their real path, size and hash.
func (m *Manager) ImportFromDisk(dir string) (*models.SyncStats, error) {
	stats := &models.SyncStats{}

	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("directory %s does not exist", dir)
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.mp3"))
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", dir, err)
	}

	// Collect the track IDs we need to enrich before hitting the feed,
	// so we only fetch pages until every on-disk track is located.
	wanted := make(map[string]bool, len(files))
	for _, path := range files {
		if id := trackIDFromFilename(path); id != "" {
			wanted[id] = true
		}
	}

	// Build id -> track map from the Suno feed (paginated, stops when found).
	feed, err := m.Suno.FetchTracksForIDs(wanted)
	if err != nil {
		return nil, fmt.Errorf("fetch feed for import: %w", err)
	}
	feedMap := make(map[string]models.Track, len(feed))
	for _, t := range feed {
		feedMap[t.ID] = t
	}

	for _, path := range files {
		id := trackIDFromFilename(path)
		if id == "" {
			stats.Errors++
			fmt.Printf("  SKIP (no id): %s\n", filepath.Base(path))
			continue
		}

		existing, err := db.GetTrack(m.DB, id)
		if err != nil {
			stats.Errors++
			continue
		}
		if existing != nil && existing.IsDownloaded {
			stats.UpdatedTracks++
			continue
		}

		track := feedMap[id]
		if track.ID == "" {
			// Not in the feed (e.g. trashed) — fall back to filename title,
			// preserving any metadata already stored for this track.
			if existing != nil {
				track = *existing
			} else {
				track.ID = id
			}
			track.Title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			if i := strings.Index(track.Title, " ["); i > 0 {
				track.Title = track.Title[:i]
			}
		} else if existing != nil {
			// The v3 feed no longer returns lyrics — keep what we already
			// extracted from the prompt for this track.
			if track.Lyrics == "" {
				track.Lyrics = existing.Lyrics
				track.LyricsPath = existing.LyricsPath
			}
		}

		track.AudioPath = path
		track.IsDownloaded = true
		if fi, err := os.Stat(path); err == nil {
			track.FileSize = fi.Size()
		}
		if hash, err := fileHash(path); err == nil {
			track.AudioHash = hash
		}

		if track.Lyrics != "" {
			if lp, err := m.saveLyrics(track); err == nil {
				track.LyricsPath = lp
				stats.LyricsExported++
			}
		}

		if err := db.UpsertTrack(m.DB, &track); err != nil {
			stats.Errors++
			fmt.Printf("  ERROR upsert %s: %v\n", id, err)
			continue
		}

		stats.TotalTracks++
		if existing == nil {
			stats.NewTracks++
		} else {
			stats.Downloaded++
		}
	}

	return stats, nil
}

// trackIDFromFilename extracts a UUID (36 chars with dashes) from a filename
// like "Title [12345678-....].mp3". Returns "" if not found.
func trackIDFromFilename(name string) string {
	re := regexp.MustCompile(`\[([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})\](?:\.mp3)?$`)
	m := re.FindStringSubmatch(filepath.Base(name))
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

// FixLyrics copies a track's prompt into its lyrics field when the prompt
// looks like song lyrics (contains section markers such as [Verse]/[Chorus])
// and the lyrics field is empty. Saves a .txt file next to the audio.
// Returns the number of tracks updated.
func (m *Manager) FixLyrics() (int, error) {
	tracks, err := db.GetAllTracks(m.DB)
	if err != nil {
		return 0, fmt.Errorf("list tracks: %w", err)
	}

	sectionMarker := regexp.MustCompile(`\[(Intro|Verse|Chorus|Pre-Chorus|Bridge|Outro|Hook|Instrumental|Speech|Rap|Build|Drop)[^]]*\]`)
	updated := 0
	for _, t := range tracks {
		// Skip tracks whose lyrics file already exists on disk.
		if t.LyricsPath != "" {
			if fi, err := os.Stat(t.LyricsPath); err == nil && !fi.IsDir() {
				continue
			}
		}

		// Fill lyrics from the prompt if empty and the prompt looks like a song.
		changed := false
		if t.Lyrics == "" && t.Prompt != "" && sectionMarker.MatchString(t.Prompt) {
			t.Lyrics = t.Prompt
			changed = true
		}

		if t.Lyrics == "" {
			continue
		}

		if t.AudioPath != "" {
			if lp, err := m.saveLyrics(t); err == nil {
				if lp != t.LyricsPath {
					t.LyricsPath = lp
					changed = true
				}
			} else {
				fmt.Printf("  WARN save lyrics for %s: %v\n", t.ID, err)
			}
		}
		if !changed {
			continue
		}
		if err := db.UpsertTrack(m.DB, &t); err != nil {
			return updated, fmt.Errorf("upsert %s: %w", t.ID, err)
		}
		updated++
	}

	return updated, nil
}

// saveLyrics writes a track's lyrics to a .txt file next to the audio.
func (m *Manager) saveLyrics(track models.Track) (string, error) {
	lyricsPath := strings.TrimSuffix(track.AudioPath, filepath.Ext(track.AudioPath)) + ".txt"
	if err := os.WriteFile(lyricsPath, []byte(track.Lyrics), 0644); err != nil {
		return "", fmt.Errorf("save lyrics: %w", err)
	}
	return lyricsPath, nil
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

// RetryDownloads re-attempts audio download for every track not yet downloaded.
// Unlike Sync it does not touch the Suno feed — it works purely from the local
// database, so it is cheap to run repeatedly until everything is archived.
func (m *Manager) RetryDownloads() (*models.SyncStats, error) {
	stats := &models.SyncStats{}

	dl := false
	tracks, err := db.ListTracks(m.DB, models.TrackFilter{Downloaded: &dl, Limit: 100000})
	if err != nil {
		return nil, fmt.Errorf("list undownloaded tracks: %w", err)
	}

	for _, track := range tracks {
		audioPath := m.audioPath(track)

		err := m.downloadTrack(track, audioPath)
		if err != nil {
			stats.Errors++
			fmt.Printf("  FAIL %s (%s): %v\n", track.ID, track.Title, err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		track.IsDownloaded = true
		track.AudioPath = audioPath
		if fi, err := os.Stat(audioPath); err == nil {
			track.FileSize = fi.Size()
		}
		if hash, err := fileHash(audioPath); err == nil {
			track.AudioHash = hash
		}

		if track.Lyrics != "" {
			if lp, err := m.saveLyrics(track); err == nil {
				track.LyricsPath = lp
				stats.LyricsExported++
			}
		}

		if err := db.UpsertTrack(m.DB, &track); err != nil {
			stats.Errors++
			continue
		}

		stats.Downloaded++
		stats.TotalTracks++
		fmt.Printf("  OK   %s (%s)\n", track.ID, track.Title)
	}

	return stats, nil
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

// effectiveAuthToken returns the real Suno auth JWT to use for API calls.
// Both the session cookie (sent by the Chrome extension) and the legacy
// auth_token field can hold a Clerk JWT. The one that expires latest wins,
// so a stale/revoked session cookie never shadows a fresher token. Tokens
// that cannot be parsed fall back to the session cookie, as before.
func effectiveAuthToken(cfg *Config) string {
	cookie, token := cfg.Suno.SessionCookie, cfg.Suno.AuthToken
	if cookie == "" {
		return token
	}
	if token == "" {
		return cookie
	}

	cookieExp, cookieOK := jwtExpiry(cookie)
	tokenExp, tokenOK := jwtExpiry(token)
	switch {
	case cookieOK && !tokenOK:
		return cookie
	case tokenOK && !cookieOK:
		return token
	case cookieOK && tokenOK && tokenExp > cookieExp:
		return token
	default:
		return cookie
	}
}

// jwtExpiry decodes a JWT's exp claim without validating the signature.
// Returns ok=false when the token is not a parseable JWT.
func jwtExpiry(jwt string) (int64, bool) {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return 0, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return 0, false
	}
	return claims.Exp, claims.Exp > 0
}

// sanitizeFilename removes characters unsafe for filenames on common
// filesystems (FAT32, NTFS, ext4): control chars and the set that FAT32
// forbids in names. Invalid chars are replaced with an underscore.
func sanitizeFilename(name string) string {
	if name == "" {
		return name
	}
	result := make([]rune, 0, len(name))
	for _, r := range name {
		if r < 0x20 || r == 0x7f || r == '/' || r == '\\' || r == ':' ||
			r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			result = append(result, '_')
		} else {
			result = append(result, r)
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
