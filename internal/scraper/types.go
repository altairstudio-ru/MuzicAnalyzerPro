package scraper

import "time"

// TrackMeta holds metadata extracted from a Suno track page.
type TrackMeta struct {
	Lyrics   string
	Prompt   string
	Tags     []string
	Title    string
	Artist   string
	Duration int
	Metadata map[string]string
	Error    string // non-empty if scraping partially failed
}

// ScrapeConfig configures the scraper behavior.
type ScrapeConfig struct {
	CDPEndpoint  string        // e.g., "ws://127.0.0.1:9222"
	AuthToken    string        // Clerk JWT (access token)
	SessionCookie string       // Clerk session cookie from browser localStorage (__session)
	DelayBetween time.Duration // pause between requests
	Timeout      time.Duration // per-request timeout
	Headless     bool          // run in headless mode
	MaxRetries   int           // number of retry attempts
}

// DefaultScrapeConfig returns a sensible default configuration.
func DefaultScrapeConfig() ScrapeConfig {
	return ScrapeConfig{
		CDPEndpoint:  "ws://127.0.0.1:9222",
		DelayBetween: 3 * time.Second,
		Timeout:      30 * time.Second,
		Headless:     true,
		MaxRetries:   2,
	}
}