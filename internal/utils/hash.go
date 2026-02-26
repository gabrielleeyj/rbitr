package utils

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"os"
	"strings"
)

func HashString(input string) string {
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])
}

func HashStringHMAC(input, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(input))
	return hex.EncodeToString(mac.Sum(nil))
}

// SecureCompare performs a constant-time comparison of two strings.
// Returns true if the strings are equal. This prevents timing attacks
// when comparing sensitive values like token hashes.
func SecureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

const (
	KeyPrefix       = "rbtr_live_"
	keyEntropyBytes = 32

	//nolint:gosec // #nosec G101 -- constant is an environment variable name, not a credential.
	TenantKeyHMACSecretsEnv = "RBTR_TENANT_KEY_HMAC_SECRETS"
)

// GenerateAPIKey creates a new API key with the rbtr_live_ prefix and 32 bytes of entropy.
// Returns the raw key (shown once) and its tenant-key hash (stored in DB).
// If RBTR_TENANT_KEY_HMAC_SECRETS is configured, hashes use HMAC-SHA256 with
// the first secret; otherwise it falls back to SHA-256.
func GenerateAPIKey() (rawKey, keyHash, prefix string, err error) {
	var buf [keyEntropyBytes]byte
	if _, err = rand.Read(buf[:]); err != nil {
		return "", "", "", err
	}
	rawKey = KeyPrefix + base64.RawURLEncoding.EncodeToString(buf[:])
	keyHash = HashTenantKey(rawKey)
	//nolint:mnd // Prefix for display: first 12 chars (rbtr_live_ + 2 chars of entropy).
	prefix = rawKey[:min(14, len(rawKey))]
	return rawKey, keyHash, prefix, nil
}

type TenantKeyHashCandidates struct {
	Current  string
	Previous []string
	Legacy   string
}

func HashTenantKey(rawKey string) string {
	candidates := BuildTenantKeyHashCandidates(rawKey, TenantKeyHMACSecretsFromEnv())
	if candidates.Current != "" {
		return candidates.Current
	}
	return candidates.Legacy
}

func TenantKeyHashCandidatesFromEnv(rawKey string) TenantKeyHashCandidates {
	return BuildTenantKeyHashCandidates(rawKey, TenantKeyHMACSecretsFromEnv())
}

func BuildTenantKeyHashCandidates(rawKey string, secrets []string) TenantKeyHashCandidates {
	out := TenantKeyHashCandidates{
		Legacy: HashString(rawKey),
	}
	cleanSecrets := normalizeSecrets(secrets)
	if len(cleanSecrets) == 0 {
		return out
	}
	out.Current = HashStringHMAC(rawKey, cleanSecrets[0])
	for _, secret := range cleanSecrets[1:] {
		hash := HashStringHMAC(rawKey, secret)
		if hash == out.Current {
			continue
		}
		out.Previous = append(out.Previous, hash)
	}
	return out
}

func TenantKeyHMACSecretsFromEnv() []string {
	return normalizeSecrets(strings.Split(strings.TrimSpace(os.Getenv(TenantKeyHMACSecretsEnv)), ","))
}

func normalizeSecrets(secrets []string) []string {
	if len(secrets) == 0 {
		return nil
	}
	out := make([]string, 0, len(secrets))
	seen := make(map[string]struct{}, len(secrets))
	for _, secret := range secrets {
		trimmed := strings.TrimSpace(secret)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}
