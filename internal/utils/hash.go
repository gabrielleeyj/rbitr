package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
)

func HashString(input string) string {
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])
}

// SecureCompare performs a constant-time comparison of two strings.
// Returns true if the strings are equal. This prevents timing attacks
// when comparing sensitive values like token hashes.
func SecureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

const (
	KeyPrefix    = "rbtr_live_"
	keyEntropyBytes = 32
)

// GenerateAPIKey creates a new API key with the rbtr_live_ prefix and 32 bytes of entropy.
// Returns the raw key (shown once) and its SHA-256 hash (stored in DB).
func GenerateAPIKey() (rawKey, keyHash, prefix string, err error) {
	buf := make([]byte, keyEntropyBytes)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", err
	}
	rawKey = KeyPrefix + base64.RawURLEncoding.EncodeToString(buf)
	keyHash = HashString(rawKey)
	// Prefix for display: first 12 chars (rbtr_live_ + 2 chars of entropy)
	prefix = rawKey[:min(14, len(rawKey))]
	return rawKey, keyHash, prefix, nil
}
