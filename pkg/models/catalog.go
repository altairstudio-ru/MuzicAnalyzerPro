package models

// Album represents a curated collection of tracks (album, compilation, EP, ...).
type Album struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Kind       string `json:"kind"`
	Notes      string `json:"notes"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	TrackCount int    `json:"track_count"`
}

// AlbumTrackItem is a track within an ordered album tracklist.
type AlbumTrackItem struct {
	Track    Track  `json:"track"`
	Position int    `json:"position"`
	Notes    string `json:"notes"`
}

// AlbumWithTracks is an album together with its ordered tracklist.
type AlbumWithTracks struct {
	Album         Album            `json:"album"`
	Tracks        []AlbumTrackItem `json:"tracks"`
	TotalDuration int              `json:"total_duration"`
}

// Label is a user-defined curation mark/label applied to tracks.
type Label struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Color      string `json:"color"`
	CreatedAt  string `json:"created_at"`
	TrackCount int    `json:"track_count"`
}

// VariantGroup groups alternative versions of the same song so analysis can
// pick the best one (best_track_id).
type VariantGroup struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Notes       string `json:"notes"`
	BestTrackID string `json:"best_track_id"`
	TrackCount  int    `json:"track_count"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// VariantGroupDetail is a variant group with its member tracks.
type VariantGroupDetail struct {
	Group  VariantGroup `json:"group"`
	Tracks []Track      `json:"tracks"`
}

// VariantSuggestion is an auto-detected set of tracks sharing the same title —
// a hint that they are alternative versions of one song.
type VariantSuggestion struct {
	Title         string   `json:"title"`
	TrackIDs      []string `json:"track_ids"`
	AllDownloaded bool     `json:"all_downloaded"`
}
