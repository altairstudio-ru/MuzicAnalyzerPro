package scraper

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// SunoScraper scrapes Suno track pages for lyrics, prompts, and metadata.
type SunoScraper struct {
	config    ScrapeConfig
	allocCtx  context.Context
	allocCancel context.CancelFunc
}

// NewSunoScraper creates a new SunoScraper with the given config.
func NewSunoScraper(cfg ScrapeConfig) *SunoScraper {
	if cfg.CDPEndpoint == "" {
		cfg = DefaultScrapeConfig()
	}
	return &SunoScraper{config: cfg}
}

// Start initializes the CDP connection to Lightpanda.
func (s *SunoScraper) Start(ctx context.Context) error {
	allocOpts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.DisableGPU,
		chromedp.UserAgent("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36"),
	}

	// Create the allocator context but don't connect yet
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), allocOpts...)
	s.allocCtx = allocCtx
	s.allocCancel = cancel

	return nil
}

// Stop cleans up the scraper resources.
func (s *SunoScraper) Stop() {
	if s.allocCancel != nil {
		s.allocCancel()
	}
}

// ScrapeTrack scrapes a single track by ID using Lightpanda CDP.
func (s *SunoScraper) ScrapeTrack(ctx context.Context, trackID string) (TrackMeta, error) {
	meta := TrackMeta{}

	if s.allocCtx == nil {
		return meta, fmt.Errorf("scraper not started, call Start() first")
	}

	log.Printf("[scraper] Scraping track %s", trackID)

	// Create a context with timeout
	taskCtx, cancel := context.WithTimeout(s.allocCtx, s.config.Timeout)
	defer cancel()

	// Create chromedp context connected to Lightpanda CDP
	log.Printf("[scraper] Connecting to CDP at %s", s.config.CDPEndpoint)
	taskCtx, cancel = chromedp.NewRemoteAllocator(s.allocCtx, s.config.CDPEndpoint)
	defer cancel()

	log.Printf("[scraper] Creating chromedp context")
	taskCtx, cancel = chromedp.NewContext(taskCtx)
	defer cancel()

	// Set Suno cookies if we have auth token
	if s.config.AuthToken != "" {
		log.Printf("[scraper] Setting Suno cookies")
		s.setSunoCookies(taskCtx)
	}

	trackURL := fmt.Sprintf("https://suno.com/song/%s", trackID)
	log.Printf("[scraper] Navigating to %s", trackURL)

	var pageHTML string
	err := chromedp.Run(taskCtx,
		chromedp.Navigate(trackURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(3*time.Second),
		chromedp.OuterHTML("html", &pageHTML, chromedp.ByQuery),
	)
	if err != nil {
		meta.Error = fmt.Sprintf("navigation failed: %v", err)
		log.Printf("[scraper] Navigation failed for track %s: %v", trackID, err)
		return meta, nil
	}

	log.Printf("[scraper] Got HTML length: %d", len(pageHTML))

	// Extract from HTML first
	meta = s.extractFromHTML(pageHTML, trackID)

	// Try DOM queries for missing fields
	if meta.Lyrics == "" || meta.Prompt == "" {
		log.Printf("[scraper] Trying DOM queries for missing fields")
		s.extractFromDOM(taskCtx, &meta)
	}

	log.Printf("[scraper] Scrape complete for %s: lyrics=%d, prompt=%d", trackID, len(meta.Lyrics), len(meta.Prompt))
	return meta, nil
}

// setSunoCookies sets the Clerk session cookies for Suno authentication.
func (s *SunoScraper) setSunoCookies(ctx context.Context) {
	if s.config.AuthToken == "" {
		return
	}

	log.Printf("[scraper] Setting Clerk session cookies")

	// The Clerk JWT is typically used as the session token
	// We need to set multiple cookies that Suno/Clerk expects
	cookies := []struct {
		name  string
		value string
	}{
		{"__session", s.config.AuthToken},
		{"__clerk_client_jwt", s.config.AuthToken},
		{"clerk_session", s.config.AuthToken},
		{"__clerk_session", s.config.AuthToken},
		{"clerk.jwt", s.config.AuthToken},
	}

	for _, c := range cookies {
		err := chromedp.Run(ctx, network.SetCookie(c.name, c.value).
			WithDomain("suno.com").
			WithPath("/").
			WithSecure(true).
			WithHTTPOnly(true).
			WithSameSite(network.CookieSameSiteLax),
		)
		if err != nil {
			log.Printf("[scraper] Failed to set cookie %s: %v", c.name, err)
		}
	}

	// Also try setting for auth.suno.com
	for _, c := range cookies {
		chromedp.Run(ctx, network.SetCookie(c.name, c.value).
			WithDomain("auth.suno.com").
			WithPath("/").
			WithSecure(true).
			WithHTTPOnly(true).
			WithSameSite(network.CookieSameSiteLax),
		)
	}
}

// extractFromHTML parses the page HTML for metadata.
func (s *SunoScraper) extractFromHTML(html, trackID string) TrackMeta {
	meta := TrackMeta{}

	// Lyrics - try multiple patterns
	lyricsPatterns := []string{
		`"lyrics"\s*:\s*"([^"]*)"`,
		`'lyrics'\s*:\s*'([^']*)'`,
		`"lyrics":\s*"([^"]*)"`,
	}
	for _, pattern := range lyricsPatterns {
		if found := extractFirstMatch(html, pattern); found != "" {
			meta.Lyrics = cleanLyrics(found)
			break
		}
	}

	// Prompt
	promptPatterns := []string{
		`"prompt"\s*:\s*"([^"]*)"`,
		`'prompt'\s*:\s*'([^']*)'`,
		`"prompt":\s*"([^"]*)"`,
	}
	for _, pattern := range promptPatterns {
		if found := extractFirstMatch(html, pattern); found != "" {
			meta.Prompt = strings.TrimSpace(found)
			break
		}
	}

	// Title
	if found := extractFirstMatch(html, `"title"\s*:\s*"([^"]*)"`); found != "" {
		meta.Title = strings.TrimSpace(found)
	} else if found := extractFirstMatch(html, `<title>([^<]+)</title>`); found != "" {
		meta.Title = strings.TrimSpace(strings.Replace(found, " | Suno", "", 1))
	}

	// Artist/handle
	if found := extractFirstMatch(html, `"handle"\s*:\s*"([^"]*)"`); found != "" {
		meta.Artist = strings.TrimSpace(found)
	} else if found := extractFirstMatch(html, `"artist"\s*:\s*"([^"]*)"`); found != "" {
		meta.Artist = strings.TrimSpace(found)
	}

	// Tags
	if found := extractFirstMatch(html, `"tags"\s*:\s*\[([^\]]*)\]`); found != "" {
		meta.Tags = parseTags(found)
	} else if found := extractFirstMatch(html, `"tags_array"\s*:\s*\[([^\]]*)\]`); found != "" {
		meta.Tags = parseTags(found)
	}

	// Duration
	if found := extractFirstMatch(html, `"duration"\s*:\s*(\d+)`); found != "" {
		var dur int
		fmt.Sscanf(found, "%d", &dur)
		meta.Duration = dur
	}

	return meta
}

// extractFromDOM queries the DOM directly for missing fields.
func (s *SunoScraper) extractFromDOM(ctx context.Context, meta *TrackMeta) {
	// Try to find lyrics
	lyricsSelectors := []string{
		`[data-testid="lyrics"]`,
		`[data-lyrics]`,
		`.lyrics`,
		`[data-testid="song-lyrics"]`,
		`section[aria-label*="lyrics" i]`,
	}
	for _, sel := range lyricsSelectors {
		var text string
		err := chromedp.Run(ctx,
			chromedp.Text(sel, &text, chromedp.ByQuery, chromedp.AtLeast(0)),
		)
		if err == nil && strings.TrimSpace(text) != "" {
			meta.Lyrics = cleanLyrics(text)
			break
		}
	}

	// Prompt
	promptSelectors := []string{
		`[data-testid="prompt"]`,
		`[data-prompt]`,
		`.prompt`,
	}
	for _, sel := range promptSelectors {
		var text string
		err := chromedp.Run(ctx,
			chromedp.Text(sel, &text, chromedp.ByQuery, chromedp.AtLeast(0)),
		)
		if err == nil && strings.TrimSpace(text) != "" {
			meta.Prompt = strings.TrimSpace(text)
			break
		}
	}

	// Title
	var title string
	err := chromedp.Run(ctx, chromedp.Text("h1", &title, chromedp.ByQuery))
	if err == nil {
		meta.Title = strings.TrimSpace(title)
	}

	// Artist
	var artist string
	err = chromedp.Run(ctx,
		chromedp.Text(`[data-testid="artist-name"], .artist-name, [data-handle]`, &artist, chromedp.ByQuery),
	)
	if err == nil {
		meta.Artist = strings.TrimSpace(artist)
	}
}

// extractFirstMatch returns the first capture group from a regex pattern.
func extractFirstMatch(text, pattern string) string {
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// cleanLyrics cleans up extracted lyrics text.
func cleanLyrics(text string) string {
	text = strings.ReplaceAll(text, `\n`, "\n")
	text = strings.ReplaceAll(text, `\r`, "\r")
	text = strings.ReplaceAll(text, `\t`, "\t")
	text = strings.ReplaceAll(text, `\"`, `"`)
	text = strings.ReplaceAll(text, `\'`, `'`)
	text = strings.ReplaceAll(text, `\\`, `\`)

	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, "\n")
}

// parseTags parses a comma-separated tags string.
func parseTags(tagsStr string) []string {
	var tags []string
	parts := strings.Split(tagsStr, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"`)
		p = strings.Trim(p, `'`)
		if p != "" {
			tags = append(tags, p)
		}
	}
	return tags
}