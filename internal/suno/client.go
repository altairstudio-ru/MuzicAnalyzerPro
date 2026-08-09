package suno

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	defaultBaseURL = "https://studio-api-prod.suno.com"
	userAgent      = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"
)

// Client communicates with the Suno private API.
type Client struct {
	baseURL    string
	httpClient *http.Client

	mu        sync.RWMutex
	authToken string
}

// NewClient creates a new Suno API client with the given auth token.
func NewClient(authToken string) *Client {
	return &Client{
		baseURL:   defaultBaseURL,
		authToken: authToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetAuthToken updates the auth token used for API requests.
// It lets a running client pick up a fresh session cookie without a restart.
func (c *Client) SetAuthToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.authToken = token
}

// getAuthToken returns the current auth token, safe for concurrent use.
func (c *Client) getAuthToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.authToken
}

// doRequest performs an HTTP request with proper auth headers.
// body is the raw request body (nil for no body); it is re-created for every
// retry attempt. Retries on rate limiting (HTTP 429) with exponential backoff.
func (c *Client) doRequest(method, path string, body []byte) (*http.Response, error) {
	url := c.baseURL + path

	const maxRetries = 6
	baseDelay := 5 * time.Second

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest(method, url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.getAuthToken())
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("do request: %w", err)
		}

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			resp.Body.Close()
			return nil, ErrUnauthorized
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			lastErr = ErrRateLimited

			// Respect Retry-After header if provided, else exponential backoff.
			delay := baseDelay << attempt
			if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
				if secs, err := time.ParseDuration(retryAfter + "s"); err == nil && secs > 0 {
					delay = secs
				}
			}
			if attempt < maxRetries {
				time.Sleep(delay)
				continue
			}
			return nil, lastErr
		}

		if resp.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
		}

		return resp, nil
	}

	return nil, lastErr
}

// ValidateToken checks if the auth token is valid by making a lightweight API call.
func (c *Client) ValidateToken() error {
	resp, err := c.doRequest("GET", "/api/feed/?page=0&page_size=1", nil)
	if err != nil {
		return fmt.Errorf("validate token: %w", err)
	}
	defer resp.Body.Close()
	return nil
}
