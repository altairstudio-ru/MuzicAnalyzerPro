package models

// Track represents a Suno track with its metadata and local storage info.
type Track struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Artist       string   `json:"artist"`
	Prompt       string   `json:"prompt"`
	Lyrics       string   `json:"lyrics"`
	Tags         []string `json:"tags"`
	Workspace    string   `json:"workspace"`
	Duration     int      `json:"duration"`
	CreatedAt    string   `json:"created_at"`
	AudioPath    string   `json:"audio_path"`
	AudioHash    string   `json:"audio_hash"`
	IsDownloaded bool     `json:"is_downloaded"`
	FileSize     int64    `json:"file_size"`
}

// Workspace represents a Suno workspace/collection.
type Workspace struct {
	Name       string `json:"name"`
	TrackCount int    `json:"track_count"`
	SyncedAt   string `json:"synced_at"`
}

// TrackFilter holds optional filter criteria for listing tracks.
type TrackFilter struct {
	Workspace  string
	Tag        string
	Search     string // search in title, prompt, lyrics
	Downloaded *bool  // nil = all, true/false filter
	Limit      int
	Offset     int
}

// SyncStats holds statistics from a sync operation.
type SyncStats struct {
	TotalTracks   int `json:"total_tracks"`
	NewTracks     int `json:"new_tracks"`
	UpdatedTracks int `json:"updated_tracks"`
	Downloaded    int `json:"downloaded"`
	Errors        int `json:"errors"`
}
