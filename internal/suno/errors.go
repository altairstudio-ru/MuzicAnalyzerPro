package suno

import "errors"

var (
	// ErrUnauthorized is returned when the auth token is invalid or expired.
	ErrUnauthorized = errors.New("suno: unauthorized — check your auth token")

	// ErrRateLimited is returned when Suno is rate-limiting requests.
	ErrRateLimited = errors.New("suno: rate limited — try again later")

	// ErrNotFound is returned when a track is not found.
	ErrNotFound = errors.New("suno: track not found")
)
