package suno

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

// apiTrack represents the raw Suno API response for a track.
// Field mapping is best-effort based on the undocumented API.
type apiTrack struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	DisplayName     string `json:"display_name"`
	SongName        string `json:"song_name"`
	Artist          string `json:"artist"`
	ArtistName      string `json:"artist_name"`
	Prompt          string `json:"prompt"`
	Lyrics          string `json:"lyrics"`
	LyricsGenerated string `json:"lyrics_generated"`
	Tags            string `json:"tags"`          // comma-separated or JSON
	TagsArray       []any  `json:"tags_array"`    // if it comes as array
	WorkspaceName   string `json:"workspace_name"`
	Workspace       string `json:"workspace"`     // sometimes nested
	Duration        int    `json:"duration"`
	CreatedAt       string `json:"created_at"`
	CreatedAtRaw    string `json:"createdAt"`     // alternative casing
	AudioURL        string `json:"audio_url"`
	AudioPath       string `json:"audio_path"`    // alternative: CDN path
	IsPublic        bool   `json:"is_public"`
	Status          string `json:"status"`        // "complete", "generating"
}

// FetchTracksResponse is the paginated response from the feed endpoint.
type FetchTracksResponse struct {
	Tracks []models.Track `json:"tracks"`
	Next   string         `json:"next"`     // cursor for next page
	HasMore bool          `json:"has_more"` // whether more pages exist
	Page    int           `json:"page"`
}

// FetchTracks retrieves the user's tracks from Suno.
// It handles pagination and converts API responses to our models.
func (c *Client) FetchTracks(page int, pageSize int) (*FetchTracksResponse, error) {
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 50
	}

	path := fmt.Sprintf("/api/feed/?page=%d&page_size=%d", page, pageSize)
	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch tracks: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// The API may return either a JSON array or an object with a results array
	var apiTracks []apiTrack
	if err := json.Unmarshal(body, &apiTracks); err != nil {
		// Try wrapped response
		var wrapped struct {
			Results []apiTrack `json:"results"`
			Total   int        `json:"total"`
			Page    int        `json:"page"`
			HasMore bool       `json:"has_more"`
		}
		if err2 := json.Unmarshal(body, &wrapped); err2 != nil {
			// Try feed container
			var feedWrapper struct {
				Feed []apiTrack `json:"feed"`
			}
			if err3 := json.Unmarshal(body, &feedWrapper); err3 != nil {
				// Return raw parse error with first attempt
				return nil, fmt.Errorf("parse tracks response: %w (body: %s)", err, truncate(string(body), 500))
			}
			apiTracks = feedWrapper.Feed
		} else {
			result := &FetchTracksResponse{
				Tracks: convertTracks(wrapped.Results),
				Next:   fmt.Sprintf("%d", wrapped.Page+1),
				HasMore: wrapped.HasMore,
				Page:    wrapped.Page,
			}
			return result, nil
		}
	}

	result := &FetchTracksResponse{
		Tracks: convertTracks(apiTracks),
		Next:   fmt.Sprintf("%d", page+1),
		HasMore: len(apiTracks) >= pageSize,
		Page:    page,
	}
	return result, nil
}

// FetchAllTracks retrieves ALL tracks by paginating through the feed.
func (c *Client) FetchAllTracks() ([]models.Track, error) {
	var allTracks []models.Track
	page := 0
	pageSize := 50

	for {
		resp, err := c.FetchTracks(page, pageSize)
		if err != nil {
			return nil, fmt.Errorf("fetch page %d: %w", page, err)
		}
		allTracks = append(allTracks, resp.Tracks...)

		if !resp.HasMore || len(resp.Tracks) == 0 {
			break
		}
		page++
	}

	return allTracks, nil
}

// FetchTrackMetadata retrieves a single track's metadata.
func (c *Client) FetchTrackMetadata(trackID string) (*models.Track, error) {
	path := fmt.Sprintf("/api/feed/%s", trackID)
	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch track metadata: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var t apiTrack
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, fmt.Errorf("parse track metadata: %w", err)
	}

	track := convertTrack(t)
	return &track, nil
}

// convertTracks converts a slice of API tracks to model Tracks.
func convertTracks(apiTracks []apiTrack) []models.Track {
	tracks := make([]models.Track, 0, len(apiTracks))
	for _, t := range apiTracks {
		tracks = append(tracks, convertTrack(t))
	}
	return tracks
}

// convertTrack converts a single API track to a model Track.
func convertTrack(t apiTrack) models.Track {
	return models.Track{
		ID:        t.ID,
		Title:     firstNonEmpty(t.Title, t.DisplayName, t.SongName),
		Artist:    firstNonEmpty(t.Artist, t.ArtistName),
		Prompt:    t.Prompt,
		Lyrics:    firstNonEmpty(t.Lyrics, t.LyricsGenerated),
		Tags:      parseTags(t.Tags, t.TagsArray),
		Workspace: firstNonEmpty(t.Workspace, t.WorkspaceName),
		Duration:  t.Duration,
		CreatedAt: firstNonEmpty(t.CreatedAt, t.CreatedAtRaw),
	}
}

// parseTags handles tags in various formats (comma-separated or JSON array).
func parseTags(tagsStr string, tagsArr []any) []string {
	// If JSON array provided
	if len(tagsArr) > 0 {
		var result []string
		for _, v := range tagsArr {
			if s, ok := v.(string); ok && s != "" {
				result = append(result, s)
			}
		}
		if len(result) > 0 {
			return result
		}
	}

	// Try JSON array format in string
	tagsStr = strings.TrimSpace(tagsStr)
	if tagsStr == "" {
		return []string{}
	}

	if strings.HasPrefix(tagsStr, "[") {
		var arr []string
		if err := json.Unmarshal([]byte(tagsStr), &arr); err == nil {
			return arr
		}
	}

	// Fallback: comma-separated
	var result []string
	for _, tag := range strings.Split(tagsStr, ",") {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			result = append(result, tag)
		}
	}
	return result
}

// firstNonEmpty returns the first non-empty string from the list.
func firstNonEmpty(strs ...string) string {
	for _, s := range strs {
		if s != "" {
			return s
		}
	}
	return ""
}

// truncate truncates a string to maxLen bytes for error messages.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
