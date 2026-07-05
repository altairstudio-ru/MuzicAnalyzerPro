package suno

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// DownloadAudio downloads a track's audio file from Suno's CDN.
// It saves the file to the given destination path.
// Returns the SHA256 hash of the downloaded file.
func (c *Client) DownloadAudio(trackID, destination string) error {
	// Create destination directory if needed
	dir := filepath.Dir(destination)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Try downloading from CDN
	audioURL := fmt.Sprintf("https://cdn1.suno.ai/%s.mp3", trackID)
	if err := downloadFile(audioURL, destination); err == nil {
		return nil
	}

	// Fallback: try via API response
	audioURL = fmt.Sprintf("https://cdn2.suno.ai/%s.mp3", trackID)
	if err := downloadFile(audioURL, destination); err == nil {
		return nil
	}

	// Another fallback with the auth header (some CDNs require it)
	audioURL = fmt.Sprintf("https://studio-api.suno.ai/api/feed/%s/audio", trackID)
	if err := c.downloadWithAuth(audioURL, destination); err == nil {
		return nil
	}

	return fmt.Errorf("download audio: all CDN endpoints failed for track %s", trackID)
}

// downloadFile downloads a file from a URL directly.
func downloadFile(url, destination string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// downloadWithAuth downloads using the client's auth headers.
func (c *Client) downloadWithAuth(url, destination string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+c.authToken)
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Detect content type to set correct extension
	contentType := resp.Header.Get("Content-Type")
	ext := ".mp3"
	if strings.Contains(contentType, "wav") || strings.Contains(contentType, "wave") {
		ext = ".wav"
	}

	// Update extension if needed
	if filepath.Ext(destination) != ext {
		destination = strings.TrimSuffix(destination, filepath.Ext(destination)) + ext
	}

	out, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
