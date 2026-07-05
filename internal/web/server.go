package web

import (
	"database/sql"
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/db"
	"github.com/altairstudio-ru/MuzicAnalyzerPro/internal/library"
	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

//go:embed templates/*.html
var templateFS embed.FS

// Server is the web UI server.
type Server struct {
	Router  *chi.Mux
	Manager *library.Manager
	DB      *sql.DB
	Tmpl    *template.Template
}

// pageData holds common data available to all templates.
type pageData struct {
	TrackCount int
	DLCount    int
	Workspaces []models.Workspace
	Tracks     []models.Track
	Filter     models.TrackFilter
	Search     string
}

// NewServer creates a new web server with the given library manager.
func NewServer(mgr *library.Manager) (*Server, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	s := &Server{
		Router:  chi.NewRouter(),
		Manager: mgr,
		DB:      mgr.DB,
		Tmpl:    tmpl,
	}

	s.Router.Use(middleware.Logger)
	s.Router.Use(middleware.Recoverer)

	s.Router.Get("/", s.dashboard)
	s.Router.Get("/tracks/{id}", s.trackDetail)
	s.Router.Post("/sync", s.triggerSync)
	s.Router.Get("/audio/{id}", s.serveAudio)
	s.Router.Post("/api/auth", s.authHandler)
	s.Router.Options("/api/auth", s.authCORS)

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

	data := pageData{
		TrackCount: trackCount,
		DLCount:    dlCount,
		Workspaces: workspaces,
		Tracks:     []models.Track{*track},
	}

	s.render(w, "detail.html", data)
}

// triggerSync starts a sync operation.
func (s *Server) triggerSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Run sync in background
	go func() {
		stats, err := s.Manager.Sync()
		if err != nil {
			log.Printf("Sync error: %v", err)
			return
		}
		log.Printf("Sync complete: %+v", stats)
	}()

	// Redirect back to main page
	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
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

// authHandler receives a Clerk JWT from the browser extension.
type authRequest struct {
	Token string `json:"token"`
}

func (s *Server) authHandler(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Token == "" {
		http.Error(w, "token is required", http.StatusBadRequest)
		return
	}

	// Save token to config
	cfg := s.Manager.Config
	cfg.Suno.AuthToken = req.Token
	if err := library.SaveConfig(cfg); err != nil {
		log.Printf("Save config error: %v", err)
		http.Error(w, "failed to save token", http.StatusInternalServerError)
		return
	}

	log.Printf("Auth token received and saved from extension")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "✓ Auth token saved. Run 'suno-archiver sync' to start downloading.",
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
