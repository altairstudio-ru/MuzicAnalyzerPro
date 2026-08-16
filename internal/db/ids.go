package db

import (
	"crypto/rand"
	"encoding/hex"
)

// newID generates a short random identifier for catalog entities
// (albums, labels, variant groups). prefix keeps entity kinds distinguishable.
func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}
