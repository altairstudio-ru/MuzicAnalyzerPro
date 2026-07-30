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
	CDPEndpoint  string        // e.g., "ws://localhost:9222"
	AuthToken    string        // Clerk JWT for authentication
	DelayBetween time.Duration // pause between requests
	Timeout      time.Duration // per-request timeout
	Headless     bool          // run in headless mode
	MaxRetries   int           // number of retry attempts
}

// DefaultScrapeConfig returns a sensible default configuration.
func DefaultScrapeConfig() ScrapeConfig {
	return ScrapeConfig{
		CDPEndpoint:  "ws://localhost:9222",
		DelayBetween: 3 * time.Second,
		Timeout:      30 * time.Second,
		Headless:     true,
		MaxRetries:   2,
	}
}