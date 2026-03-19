package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/cache"
)

// Admin session errors.
var (
	ErrAdminSessionExpired = errors.New("admin session expired")
	ErrAdminSessionInvalid = errors.New("admin session invalid")
	ErrAdminSessionRevoked = errors.New("admin session revoked")
)

// AdminSessionClaims holds the claims for an SSO-authenticated admin session.
type AdminSessionClaims struct {
	SessionID string   `json:"sid"`
	Email     string   `json:"email"`
	Name      string   `json:"name"`
	Subject   string   `json:"sub"`
	Scopes    []string `json:"scopes"`
	IssuedAt  int64    `json:"iat"`
	ExpiresAt int64    `json:"exp"`
	AuthType  string   `json:"auth_type"` // "oidc"
}

const (
	// AdminSessionTTL is the default lifetime for admin SSO sessions.
	AdminSessionTTL = 8 * time.Hour

	// adminSessionSigningKeyLen is the length of the HMAC signing key in bytes.
	adminSessionSigningKeyLen = 32

	// adminSessionCachePadding adds buffer beyond token TTL for revocation tracking.
	adminSessionCachePadding = 10 * time.Minute

	// adminSessionPrefix distinguishes admin session tokens from tenant session tokens.
	adminSessionPrefix = "rbas_" // rbitr admin session
)

// AdminSessionManager creates and validates admin SSO session tokens.
type AdminSessionManager struct {
	signingKey []byte
	ttl        time.Duration
	sessions   *cache.TTLCache[AdminSessionClaims]
}

// NewAdminSessionManager creates an AdminSessionManager with a random signing key.
func NewAdminSessionManager(ttl time.Duration) (*AdminSessionManager, error) {
	key := make([]byte, adminSessionSigningKeyLen) //nolint:makezero // rand.Read requires pre-allocated slice
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate admin session signing key: %w", err)
	}

	return &AdminSessionManager{
		signingKey: key,
		ttl:        ttl,
		sessions:   cache.New[AdminSessionClaims](ttl + adminSessionCachePadding),
	}, nil
}

// newAdminSessionManagerWithKey creates an AdminSessionManager with a specific key (for testing).
func newAdminSessionManagerWithKey(key []byte, ttl time.Duration) *AdminSessionManager {
	return &AdminSessionManager{
		signingKey: key,
		ttl:        ttl,
		sessions:   cache.New[AdminSessionClaims](ttl + adminSessionCachePadding),
	}
}

// IssueSession creates a new admin session for an SSO-authenticated user.
func (m *AdminSessionManager) IssueSession(user OIDCUserInfo, scopes []string) (string, AdminSessionClaims, error) {
	sessionID, err := generateAdminSessionID()
	if err != nil {
		return "", AdminSessionClaims{}, err
	}

	now := time.Now()
	claims := AdminSessionClaims{
		SessionID: sessionID,
		Email:     user.Email,
		Name:      user.Name,
		Subject:   user.Subject,
		Scopes:    scopes,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(m.ttl).Unix(),
		AuthType:  "oidc",
	}

	token, err := m.signClaims(&claims)
	if err != nil {
		return "", AdminSessionClaims{}, err
	}

	m.sessions.Set(sessionID, claims)

	return token, claims, nil
}

// ValidateSession verifies an admin session token and returns the claims.
func (m *AdminSessionManager) ValidateSession(token string) (AdminSessionClaims, error) {
	claims, err := m.verifyClaims(token)
	if err != nil {
		return AdminSessionClaims{}, err
	}

	now := time.Now().Unix()
	if now > claims.ExpiresAt {
		return AdminSessionClaims{}, ErrAdminSessionExpired
	}

	if _, found := m.sessions.Get(claims.SessionID); !found {
		return AdminSessionClaims{}, ErrAdminSessionRevoked
	}

	return claims, nil
}

// RevokeSession invalidates an admin session by its ID.
func (m *AdminSessionManager) RevokeSession(sessionID string) {
	m.sessions.Invalidate(sessionID)
}

// IsAdminSessionToken checks if a token looks like an admin session token.
func IsAdminSessionToken(token string) bool {
	// Admin session tokens start with the payload which is base64url encoded,
	// contain a dot separator, and the decoded payload starts with a JSON object
	// containing "sid" starting with "rbas_".
	if !strings.Contains(token, ".") {
		return false
	}
	parts := strings.SplitN(token, ".", 2) //nolint:mnd // token format: payload.signature
	if len(parts) != 2 {                   //nolint:mnd // token format: payload.signature
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	return strings.Contains(string(payload), adminSessionPrefix)
}

func (m *AdminSessionManager) signClaims(claims *AdminSessionClaims) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal admin session claims: %w", err)
	}

	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := m.computeHMAC([]byte(encodedPayload))
	encodedSig := base64.RawURLEncoding.EncodeToString(signature)

	return encodedPayload + "." + encodedSig, nil
}

func (m *AdminSessionManager) verifyClaims(token string) (AdminSessionClaims, error) {
	parts := strings.SplitN(token, ".", 2) //nolint:mnd // token format: payload.signature
	if len(parts) != 2 {                   //nolint:mnd // token format: payload.signature
		return AdminSessionClaims{}, ErrAdminSessionInvalid
	}

	encodedPayload := parts[0]
	encodedSig := parts[1]

	expectedSig := m.computeHMAC([]byte(encodedPayload))
	providedSig, err := base64.RawURLEncoding.DecodeString(encodedSig)
	if err != nil {
		return AdminSessionClaims{}, ErrAdminSessionInvalid
	}
	if !hmac.Equal(expectedSig, providedSig) {
		return AdminSessionClaims{}, ErrAdminSessionInvalid
	}

	payload, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return AdminSessionClaims{}, ErrAdminSessionInvalid
	}

	var claims AdminSessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return AdminSessionClaims{}, ErrAdminSessionInvalid
	}

	return claims, nil
}

func (m *AdminSessionManager) computeHMAC(data []byte) []byte {
	mac := hmac.New(sha256.New, m.signingKey)
	mac.Write(data)
	return mac.Sum(nil)
}

func generateAdminSessionID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return adminSessionPrefix + base64.RawURLEncoding.EncodeToString(buf[:]), nil
}
