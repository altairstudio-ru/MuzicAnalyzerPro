package web

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/analyzer"
	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/db"
	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/library"
	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/scraper"
	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

//go:embed templates/*.html
var templateFS embed.FS

// Server is the web UI server.
type Server struct {
	Router      *chi.Mux
	Manager     *library.Manager
	DB          *sql.DB
	Tmpl        *template.Template
	Analyzer    *analyzer.Analyzer
	scraperCfg  scraper.ScrapeConfig
	scraper     *scraper.SunoScraper
	scraperMu   sync.Mutex
}

// pageData holds common data available to all templates.
type pageData struct {
	TrackCount     int
	DLCount        int
	Workspaces     []models.Workspace
	Tracks         []models.Track
	AllTracks      []models.Track
	Filter         models.TrackFilter
	Search         string
	AnalysisResult *db.AnalysisResult
}

// NewServer creates a new web server with the given library manager.
func NewServer(mgr *library.Manager) (*Server, error) {
	funcMap := template.FuncMap{
		"formatLyrics": formatLyrics,
	}
	tmpl := template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html"))

	// Use absolute path for Python venv
	venvPath, _ := filepath.Abs(".venv/bin/python3")
	anz := analyzer.New(mgr.DB, venvPath)

	// Scraper config (lazy init)
	scraperCfg := scraper.DefaultScrapeConfig()
	scraperCfg.AuthToken = mgr.Config.Suno.AuthToken
	scraperCfg.SessionCookie = mgr.Config.Suno.SessionCookie

	s := &Server{
		Router:      chi.NewRouter(),
		Manager:     mgr,
		DB:          mgr.DB,
		Tmpl:        tmpl,
		Analyzer:    anz,
		scraperCfg:  scraperCfg,
	}

	s.Router.Use(middleware.Logger)
	s.Router.Use(middleware.Recoverer)

	s.Router.Get("/", s.dashboard)
	s.Router.Get("/tracks/{id}", s.trackDetail)
	s.Router.Post("/sync", s.triggerSync)
	s.Router.Post("/sync/stop", s.stopSync)
	s.Router.Get("/api/sync-status", s.syncStatusHandler)
	s.Router.Get("/audio/{id}", s.serveAudio)
	s.Router.Get("/lyrics/{id}", s.serveLyrics)
	s.Router.Post("/export-lyrics/{id}", s.exportLyrics)
	s.Router.Post("/api/auth", s.authHandler)
	s.Router.Options("/api/auth", s.authCORS)
	s.Router.Get("/api/health", s.healthHandler)
	s.Router.Post("/analyze/{id}", s.triggerAnalyze)
	s.Router.Get("/analyze/{id}/status", s.analyzeStatus)
	s.Router.Post("/compare/upload/{id}", s.compareUpload)
	s.Router.Post("/compare/select/{id}", s.compareSelect)
	s.Router.Get("/plots/*", s.servePlot)
	s.Router.Post("/scrape-lyrics/{id}", s.scrapeLyricsHandler)

	return s, nil
}

// dashboard shows the main page with stats, workspaces, and track list.
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	trackCount, _ := db.GetTrackCount(s.DB)
	dlCount, _ := db.GetDownloadedCount(s.DB)
	workspaces, _ := db.ListWorkspaces(s.DB)

	filter := models.TrackFilter{
		Workspace: r.URL.Query().Get("workspace"),
		Search:    r.URL.Query().Get("search"),
		Limit:     100,
	}

	tracks, err := db.ListTracks(s.DB, filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := pageData{
		TrackCount: trackCount,
		DLCount:    dlCount,
		Workspaces: workspaces,
		Tracks:     tracks,
		Filter:     filter,
		Search:     filter.Search,
	}

	s.render(w, "index.html", data)
}

// trackDetail shows a single track's full metadata.
func (s *Server) trackDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	track, err := db.GetTrack(s.DB, id)
	if err != nil || track == nil {
		http.Error(w, "Track not found", http.StatusNotFound)
		return
	}

	trackCount, _ := db.GetTrackCount(s.DB)
	dlCount, _ := db.GetDownloadedCount(s.DB)
	workspaces, _ := db.ListWorkspaces(s.DB)

	analysisResult, _ := db.GetAnalysisResult(s.DB, id)

	allTracks, _ := db.ListTracks(s.DB, models.TrackFilter{Limit: 200})

	data := pageData{
		TrackCount:     trackCount,
		DLCount:        dlCount,
		Workspaces:     workspaces,
		Tracks:         []models.Track{*track},
		AllTracks:      allTracks,
		AnalysisResult: analysisResult,
	}

	s.render(w, "detail.html", data)
}

// triggerSync starts a sync operation.
func (s *Server) triggerSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Atomically reserve the sync slot. If a sync is already running, the
	// background goroutine from TrySyncBackground is the only one allowed.
	if !s.Manager.TrySyncBackground() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": "sync already in progress"})
		return
	}

	// Accepted — the frontend polls /api/sync-status for progress.
	w.WriteHeader(http.StatusAccepted)
}

// stopSync requests the running background sync to stop at the next safe point.
func (s *Server) stopSync(w http.ResponseWriter, r *http.Request) {
	s.Manager.StopSync()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "stopping"})
}

// syncStatusHandler returns the live sync progress as JSON.
func (s *Server) syncStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	st := s.Manager.GetSyncStatus()
	json.NewEncoder(w).Encode(map[string]any{
		"running":     st.Running,
		"phase":       st.Phase,
		"processed":   st.Processed,
		"new":         st.New,
		"downloaded":  st.Downloaded,
		"errors":      st.Errors,
		"last_track":  st.LastTrack,
		"started_at":  st.StartedAt,
		"finished_at": st.FinishedAt,
		"error":       st.ErrMsg,
		"stopped":     st.Stopped,
	})
}

// serveAudio serves an audio file for inline playback.
func (s *Server) serveAudio(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	track, err := db.GetTrack(s.DB, id)
	if err != nil || track == nil || !track.IsDownloaded {
		http.Error(w, "Audio not available", http.StatusNotFound)
		return
	}

	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeFile(w, r, track.AudioPath)
}

// exportLyrics runs Whisper transcription and saves it as lyrics.
func (s *Server) exportLyrics(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	track, err := db.GetTrack(s.DB, id)
	if err != nil || track == nil {
		http.Error(w, "Track not found", http.StatusNotFound)
		return
	}
	if !track.IsDownloaded || track.AudioPath == "" {
		http.Error(w, "Audio not available", http.StatusBadRequest)
		return
	}

	// Run analysis with just whisper metric
	result, err := s.Analyzer.Analyze(id, track.AudioPath, []string{"whisper"}, track.Lyrics)
	if err != nil {
		http.Error(w, "Analysis failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Extract transcription from whisper results
	var transcription string
	if resultJSON, ok := result.Results["whisper"]; ok {
		if whisperMap, ok := resultJSON.(map[string]any); ok {
			if fullText, ok := whisperMap["full_text"].(string); ok && fullText != "" {
				transcription = fullText
			}
		}
	}

	if transcription == "" {
		http.Error(w, "No transcription generated", http.StatusInternalServerError)
		return
	}

	// Save lyrics file
	lyricsPath := strings.TrimSuffix(track.AudioPath, filepath.Ext(track.AudioPath)) + ".txt"
	if err := os.WriteFile(lyricsPath, []byte(transcription), 0644); err != nil {
		http.Error(w, "Failed to save lyrics: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update track in DB
	track.LyricsPath = lyricsPath
	track.Lyrics = transcription // Also update in-memory for immediate use
	if err := db.UpsertTrack(s.DB, track); err != nil {
		http.Error(w, "DB update failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Serve the file
	downloadName := sanitizeFilename(track.Title)
	if downloadName == "" {
		downloadName = track.ID
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.txt"`, downloadName))
	http.ServeFile(w, r, lyricsPath)
}

// serveLyrics serves a track's lyrics file for download.
func (s *Server) serveLyrics(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	track, err := db.GetTrack(s.DB, id)
	if err != nil || track == nil {
		http.Error(w, "Track not found", http.StatusNotFound)
		return
	}

	if track.LyricsPath != "" {
		if fi, err := os.Stat(track.LyricsPath); err == nil && !fi.IsDir() {
			downloadName := sanitizeFilename(track.Title)
			if downloadName == "" {
				downloadName = track.ID
			}
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.txt"`, downloadName))
			http.ServeFile(w, r, track.LyricsPath)
			return
		}
	}

	if track.Lyrics != "" {
		downloadName := sanitizeFilename(track.Title)
		if downloadName == "" {
			downloadName = track.ID
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.txt"`, downloadName))
		w.Write([]byte(track.Lyrics))
		return
	}

	http.Error(w, "No lyrics available", http.StatusNotFound)
}

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

// triggerAnalyze starts analysis for a track.
func (s *Server) triggerAnalyze(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	track, err := db.GetTrack(s.DB, id)
	if err != nil || track == nil {
		http.Error(w, "Track not found", http.StatusNotFound)
		return
	}
	if !track.IsDownloaded {
		http.Error(w, "Audio not downloaded", http.StatusBadRequest)
		return
	}

	// Insert pending record immediately so the redirect shows pending status
	_ = db.UpsertAnalysisResult(s.DB, &db.AnalysisResult{
		TrackID:    id,
		Version:    1,
		Status:     "pending",
		ResultJSON: "{}",
	})

	lyrics := track.Lyrics

	// Run async
	go func() {
		_, err := s.Analyzer.Analyze(id, track.AudioPath, []string{"all"}, lyrics)
		if err != nil {
			log.Printf("[analyzer] analyze track %s: %v", id, err)
		}
	}()

	w.Header().Set("HX-Redirect", "/tracks/"+id)
	w.WriteHeader(http.StatusOK)
}

// analyzeStatus returns the analysis result JSON for a track.
func (s *Server) analyzeStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	result, err := db.GetAnalysisResult(s.DB, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if result == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "not_found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// compareUpload handles reference file upload and triggers comparison analysis.
func (s *Server) compareUpload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	track, err := db.GetTrack(s.DB, id)
	if err != nil || track == nil {
		http.Error(w, "Track not found", http.StatusNotFound)
		return
	}
	if !track.IsDownloaded {
		http.Error(w, "Audio not downloaded", http.StatusBadRequest)
		return
	}

	r.ParseMultipartForm(50 << 20) // 50MB max
	file, header, err := r.FormFile("ref_file")
	if err != nil {
		http.Error(w, "Missing ref_file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	refDir := filepath.Join(s.Manager.Config.DBDir(), "references", id)
	os.MkdirAll(refDir, 0755)
	refPath := filepath.Join(refDir, header.Filename)
	dst, err := os.Create(refPath)
	if err != nil {
		http.Error(w, "Failed to save reference", http.StatusInternalServerError)
		return
	}
	defer dst.Close()
	io.Copy(dst, file)

	s.analyzeWithReference(w, r, id, track, refPath)
}

// compareSelect triggers comparison analysis using another synced track as reference.
func (s *Server) compareSelect(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	track, err := db.GetTrack(s.DB, id)
	if err != nil || track == nil {
		http.Error(w, "Track not found", http.StatusNotFound)
		return
	}
	if !track.IsDownloaded {
		http.Error(w, "Audio not downloaded", http.StatusBadRequest)
		return
	}

	refID := r.FormValue("ref_id")
	if refID == "" {
		http.Error(w, "ref_id required", http.StatusBadRequest)
		return
	}
	refTrack, err := db.GetTrack(s.DB, refID)
	if err != nil || refTrack == nil || !refTrack.IsDownloaded {
		http.Error(w, "Reference track not found or not downloaded", http.StatusBadRequest)
		return
	}

	s.analyzeWithReference(w, r, id, track, refTrack.AudioPath)
}

func (s *Server) analyzeWithReference(w http.ResponseWriter, r *http.Request, id string, track *models.Track, refPath string) {
	_ = db.UpsertAnalysisResult(s.DB, &db.AnalysisResult{
		TrackID:    id,
		Version:    1,
		Status:     "pending",
		ResultJSON: "{}",
	})

	lyrics := track.Lyrics

	go func() {
		_, err := s.Analyzer.Analyze(id, track.AudioPath, []string{"all"}, lyrics, refPath)
		if err != nil {
			log.Printf("[analyzer] compare track %s: %v", id, err)
		}
	}()

	w.Header().Set("HX-Redirect", "/tracks/"+id)
	w.WriteHeader(http.StatusOK)
}

// scrapeLyricsHandler scrapes lyrics and metadata from Suno track page using Lightpanda.
func (s *Server) scrapeLyricsHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	track, err := db.GetTrack(s.DB, id)
	if err != nil || track == nil {
		http.Error(w, "Track not found", http.StatusNotFound)
		return
	}

	// Lazy init scraper
	s.scraperMu.Lock()
	if s.scraper == nil {
		scraper := scraper.NewSunoScraper(s.scraperCfg)
		ctx := context.Background()
		if err := scraper.Start(ctx); err != nil {
			log.Printf("[scraper] failed to start: %v", err)
		}
		s.scraper = scraper
	}
	scraper := s.scraper
	s.scraperMu.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	meta, err := scraper.ScrapeTrack(ctx, id)
	if err != nil {
		http.Error(w, "Scraping failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if meta.Error != "" {
		http.Error(w, "Scraping error: "+meta.Error, http.StatusInternalServerError)
		return
	}

	// Save lyrics file if we got lyrics
	lyricsPath := ""
	if meta.Lyrics != "" && track.AudioPath != "" {
		lyricsPath = strings.TrimSuffix(track.AudioPath, filepath.Ext(track.AudioPath)) + ".txt"
		if err := os.WriteFile(lyricsPath, []byte(meta.Lyrics), 0644); err != nil {
			log.Printf("[scraper] failed to save lyrics: %v", err)
		}
	}

	// Update track in DB
	if meta.Lyrics != "" {
		track.Lyrics = meta.Lyrics
		track.LyricsPath = lyricsPath
	}
	if meta.Prompt != "" && track.Prompt == "" {
		track.Prompt = meta.Prompt
	}
	if len(meta.Tags) > 0 && len(track.Tags) == 0 {
		track.Tags = meta.Tags
	}
	if meta.Title != "" && track.Title == "" {
		track.Title = meta.Title
	}
	if meta.Artist != "" && track.Artist == "" {
		track.Artist = meta.Artist
	}

	if err := db.UpsertTrack(s.DB, track); err != nil {
		log.Printf("[scraper] failed to update track: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success":     true,
		"lyrics":      meta.Lyrics,
		"prompt":      meta.Prompt,
		"tags":        meta.Tags,
		"title":       meta.Title,
		"artist":      meta.Artist,
		"lyrics_path": lyricsPath,
	})
}

// servePlot serves comparison plot images.
func (s *Server) servePlot(w http.ResponseWriter, r *http.Request) {
	plotPath := chi.URLParam(r, "*")
	if plotPath == "" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	plotsDir := filepath.Join(filepath.Dir(s.Analyzer.Script), "..", "plots")
	fullPath := filepath.Join(plotsDir, plotPath)
	if !strings.HasPrefix(fullPath, plotsDir) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeFile(w, r, fullPath)
}

// healthHandler returns 200 — used by the extension to check if the app is running.
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// authHandler receives a Clerk JWT from the browser extension.
type authRequest struct {
	Token          string `json:"token"`
	SessionCookie  string `json:"session_cookie"`
}

func (s *Server) authHandler(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	cfg := s.Manager.Config
	updated := false

	if req.Token != "" {
		cfg.Suno.AuthToken = req.Token
		s.Manager.Suno.SetAuthToken(req.Token)
		updated = true
	}
	if req.SessionCookie != "" {
		cfg.Suno.SessionCookie = req.SessionCookie
		s.Manager.SetAuthToken(req.SessionCookie)
		// Also update scraper config if already initialized
		s.scraperMu.Lock()
		if s.scraper != nil {
			s.scraper.SetSessionCookie(req.SessionCookie)
		}
		s.scraperCfg.SessionCookie = req.SessionCookie
		s.scraperMu.Unlock()
		updated = true
	}

	if !updated {
		http.Error(w, "token or session_cookie is required", http.StatusBadRequest)
		return
	}

	if err := library.SaveConfig(cfg); err != nil {
		log.Printf("Save config error: %v", err)
		http.Error(w, "failed to save config", http.StatusInternalServerError)
		return
	}

	log.Printf("Auth config received and saved from extension (token: %v, session_cookie: %v)",
		req.Token != "", req.SessionCookie != "")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "✓ Auth config saved. Run 'suno-archiver sync' to start downloading.",
	})
}

// authCORS handles preflight OPTIONS requests from the extension.
func (s *Server) authCORS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "chrome-extension://*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.WriteHeader(http.StatusNoContent)
}

// render executes a template with the given data.
func (s *Server) render(w http.ResponseWriter, tmpl string, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.Tmpl.ExecuteTemplate(w, tmpl, data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
	}
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe(addr string) error {
	if addr == "" {
		addr = ":8080"
	}
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}
	log.Printf("Web UI: http://localhost%s", addr)
	return http.ListenAndServe(addr, s.Router)
}

// formatLyrics formats lyrics text for HTML display, preserving line breaks.
func formatLyrics(text string) template.HTML {
	if text == "" {
		return ""
	}
	// Escape HTML, then replace newlines with <br>
	escaped := template.HTMLEscapeString(text)
	return template.HTML(strings.ReplaceAll(escaped, "\n", "<br>"))
}
