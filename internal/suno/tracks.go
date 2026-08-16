package suno

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/altairstudio-ru/MuzicAnalyzerPro/pkg/models"
)

// apiTrack represents the raw Suno API response for a track (new v3 API).
// Field mapping based on the undocumented API response.
type apiTrack struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	DisplayName     string     `json:"display_name"`
	SongName        string     `json:"song_name"`
	Artist          string     `json:"artist"`
	ArtistName      string     `json:"artist_name"`
	Prompt          string     `json:"prompt"`
	Lyrics          string     `json:"lyrics"`
	LyricsGenerated string     `json:"lyrics_generated"`
	Tags            string     `json:"tags"`       // comma-separated or JSON
	TagsArray       []any      `json:"tags_array"` // if it comes as array
	WorkspaceName   string     `json:"workspace_name"`
	Workspace       string     `json:"workspace"` // sometimes nested
	Project         apiProject `json:"project"`   // v3 API: project (a.k.a. workspace)
	Albums          []apiAlbum `json:"albums"`    // v3 API: albums the track belongs to
	Duration        int        `json:"duration"`
	CreatedAt       string     `json:"created_at"`
	CreatedAtRaw    string     `json:"createdAt"` // alternative casing
	AudioURL        string     `json:"audio_url"`
	AudioPath       string     `json:"audio_path"` // alternative: CDN path
	IsPublic        bool       `json:"is_public"`
	Status          string     `json:"status"` // "complete", "generating"

	// New v3 API fields
	PlayCount         int           `json:"play_count"`
	UpvoteCount       int           `json:"upvote_count"`
	IsLiked           bool          `json:"is_liked"`
	EntityType        string        `json:"entity_type"`
	VideoURL          string        `json:"video_url"`
	MediaURLs         []MediaURL    `json:"media_urls"`
	ImageURL          string        `json:"image_url"`
	ImageLargeURL     string        `json:"image_large_url"`
	MajorModelVersion string        `json:"major_model_version"`
	ModelName         string        `json:"model_name"`
	Metadata          TrackMetadata `json:"metadata"`
}

type MediaURL struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	Delivery    string `json:"delivery"`
	Encoding    string `json:"encoding"`
}

type TrackMetadata struct {
	Tags   string `json:"tags"`
	Prompt string `json:"prompt"`
	Type   string `json:"type"` // gen | cover | extend | ...
}

// apiProject is the v3 API's project object — the new name for a workspace.
type apiProject struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsTrashed   bool   `json:"is_trashed"`
	IsPublic    bool   `json:"is_public"`
}

// apiAlbum is a Suno album the track belongs to.
type apiAlbum struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// FetchTracksResponse is the paginated response from the feed v3 endpoint.
type FetchTracksResponse struct {
	Tracks  []models.Track `json:"tracks"`
	Next    string         `json:"next"`     // cursor for next page
	HasMore bool           `json:"has_more"` // whether more pages exist
}

// FetchTracks retrieves tracks from Suno v3 API using cursor-based pagination.
// Pass cursor="" for the first page, then use the returned Next cursor.
func (c *Client) FetchTracks(cursor string, pageSize int) (*FetchTracksResponse, error) {
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 50
	}

	path := "/api/feed/v3"
	bodyData := map[string]interface{}{
		"page_size": pageSize,
	}
	if cursor != "" {
		bodyData["cursor"] = cursor
	}

	bodyBytes, _ := json.Marshal(bodyData)
	resp, err := c.doRequest("POST", path, bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("fetch tracks: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var v3Resp struct {
		Clips      []apiTrack `json:"clips"`
		NextCursor string     `json:"next_cursor"`
		HasMore    bool       `json:"has_more"`
	}
	if err := json.Unmarshal(body, &v3Resp); err != nil {
		return nil, fmt.Errorf("parse v3 tracks response: %w (body: %s)", err, truncate(string(body), 500))
	}

	result := &FetchTracksResponse{
		Tracks:  convertTracks(v3Resp.Clips),
		Next:    v3Resp.NextCursor,
		HasMore: v3Resp.HasMore,
	}
	return result, nil
}

// FetchAllTracks retrieves ALL tracks by paginating through the feed using cursor.
func (c *Client) FetchAllTracks() ([]models.Track, error) {
	return c.fetchTracksUntil(nil)
}

// FetchTracksForIDs paginates through the feed (with a small inter-page delay)
// and returns tracks whose IDs are in the `wanted` set. Stops early once every
// wanted ID has been found, reducing the number of requests. Unknown IDs are
// simply absent from the result.
func (c *Client) FetchTracksForIDs(wanted map[string]bool) ([]models.Track, error) {
	if len(wanted) == 0 {
		return nil, nil
	}
	return c.fetchTracksUntil(wanted)
}

func (c *Client) fetchTracksUntil(wanted map[string]bool) ([]models.Track, error) {
	var allTracks []models.Track
	cursor := ""
	pageSize := 200
	found := map[string]bool{}

	for {
		resp, err := c.FetchTracks(cursor, pageSize)
		if err != nil {
			return nil, fmt.Errorf("fetch cursor %q: %w", cursor, err)
		}
		allTracks = append(allTracks, resp.Tracks...)

		// Early exit once all wanted tracks are located.
		if wanted != nil {
			for _, t := range resp.Tracks {
				if wanted[t.ID] {
					found[t.ID] = true
				}
			}
			if len(found) == len(wanted) {
				break
			}
		}

		if !resp.HasMore || len(resp.Tracks) == 0 {
			break
		}
		cursor = resp.Next

		// Be gentle with the API between pages.
		time.Sleep(800 * time.Millisecond)
	}

	return allTracks, nil
}

// FetchTrackMetadata retrieves a single track's metadata.
// Note: The v3 API doesn't have a single-track endpoint, so we fetch the feed
// and filter. This is inefficient but works for now.
func (c *Client) FetchTrackMetadata(trackID string) (*models.Track, error) {
	resp, err := c.FetchTracks("", 100)
	if err != nil {
		return nil, fmt.Errorf("fetch track metadata: %w", err)
	}
	for _, t := range resp.Tracks {
		if t.ID == trackID {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("track %s not found in recent feed", trackID)
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
	// Extract tags from metadata if available, fallback to old fields
	tags := parseTags(t.Tags, t.TagsArray)
	if len(tags) == 0 && t.Metadata.Tags != "" {
		tags = parseTags(t.Metadata.Tags, nil)
	}

	// Extract prompt from metadata if available
	prompt := t.Prompt
	if prompt == "" && t.Metadata.Prompt != "" {
		prompt = t.Metadata.Prompt
	}

	// Workspace: the v3 API names it "project"; fall back to the legacy
	// workspace/workspace_name fields for older responses.
	workspace := firstNonEmpty(t.Project.Name, t.Workspace, t.WorkspaceName)

	return models.Track{
		ID:          t.ID,
		Title:       firstNonEmpty(t.Title, t.DisplayName, t.SongName),
		Artist:      firstNonEmpty(t.Artist, t.ArtistName),
		Prompt:      prompt,
		Lyrics:      firstNonEmpty(t.Lyrics, t.LyricsGenerated),
		Tags:        tags,
		Workspace:   workspace,
		Duration:    t.Duration,
		CreatedAt:   firstNonEmpty(t.CreatedAt, t.CreatedAtRaw),
		UpvoteCount: t.UpvoteCount,
		PlayCount:   t.PlayCount,
		IsLiked:     t.IsLiked,
		TrackType:   deriveTrackType(t.Metadata.Type, t.EntityType),
		ModelName:   t.ModelName,
	}
}

// deriveTrackType maps Suno API type/entity fields to a stable local value.
// Known values from community clients: metadata.type = gen|cover|extend;
// entity_type may repeat similar strings. Unknown values stay empty.
func deriveTrackType(metaType, entityType string) string {
	raw := strings.ToLower(strings.TrimSpace(firstNonEmpty(metaType, entityType)))
	switch raw {
	case "cover":
		return "cover"
	case "extend":
		return "extend"
	case "gen", "generate", "generated", "full_song", "full-song", "fullsong", "song":
		return "full_song"
	default:
		return ""
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
