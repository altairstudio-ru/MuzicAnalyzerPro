package library

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	stats := &models.SyncStats{}

	pageSize := 50
	cursor := ""
	workspaceSet := make(map[string]bool)

	for {
		resp, err := m.Suno.FetchTracks(cursor, pageSize)
		if err != nil {
			if suno.IsRateLimited(err) && stats.TotalTracks > 0 {
				return stats, fmt.Errorf("rate limited at cursor %q (processed %d tracks)", cursor, stats.TotalTracks)
			}
			return nil, fmt.Errorf("fetch cursor %q: %w", cursor, err)
		}

		for _, track := range resp.Tracks {
			isNew := false

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
		}

		if !resp.HasMore || len(resp.Tracks) == 0 {
			break
		}
		cursor = resp.Next
		time.Sleep(500 * time.Millisecond)
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

	return stats, nil
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