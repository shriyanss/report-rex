// Package synccommon holds pure, dependency-free logic shared by the sync
// client and server: hashing, path normalization, exclude-pattern matching,
// and sync-state classification.
package synccommon

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashBytes returns the lowercase hex-encoded SHA-256 digest of b.
func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
