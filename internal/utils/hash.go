package utils

import (
	"crypto/sha256"
	"encoding/hex"
)

func HashString(input string) string {
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])
}
