package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/cache"
)

const (
	// SessionTokenHeader is the preferred header for session tokens.
	SessionTokenHeader = "X-Session-Token"

	// DefaultSessionTTL is the default lifetime for session tokens.
	DefaultSessionTTL = 15 * time.Minute

	// sessionSigningKeyLen is the length of the HMAC signing key in bytes.
	sessionSigningKeyLen = 32
)

// Session token errors.
var (
	ErrSessionExpired     = errors.New("session token expired")
	ErrSessionInvalid     = errors.New("session token invalid")
	ErrSessionIPMismatch  = errors.New("session token IP mismatch")
	ErrSessionRevoked     = errors.New("session token revoked")
	ErrSessionKeyGenerate = errors.New("failed to generate signing key")
)

// SessionClaims are the claims encoded in a session token.
type SessionClaims struct {
	TenantID  string `json:"tid"`
	AgentID   string `json:"aid"`
	SessionID string `json:"sid"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	SourceIP  string `json:"sip"`
}

// SessionManager creates and validates short-lived session tokens.
// Tokens are HMAC-SHA256 signed and bound to tenant, agent, and source IP.
type SessionManager struct {
	signingKey []byte
	ttl        time.Duration
	sessions   *cache.TTLCache[SessionClaims]
}

// sessionCachePadding adds buffer to the cache TTL beyond the token TTL so
// that explicitly-revoked sessions remain trackable until well after token expiry.
const sessionCachePadding = 5 * time.Minute

// NewSessionManager creates a SessionManager with a random signing key.
func NewSessionManager(ttl time.Duration) (*SessionManager, error) {
	key := make([]byte, sessionSigningKeyLen) //nolint:makezero // rand.Read requires pre-allocated slice
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSessionKeyGenerate, err)
	}

	return &SessionManager{
		signingKey: key,
		ttl:        ttl,
		sessions:   cache.New[SessionClaims](ttl + sessionCachePadding),
	}, nil
}

// newSessionManagerWithKey creates a SessionManager with a specific key (for testing).
func newSessionManagerWithKey(key []byte, ttl time.Duration) *SessionManager {
	return &SessionManager{
		signingKey: key,
		ttl:        ttl,
		sessions:   cache.New[SessionClaims](ttl + sessionCachePadding),
	}
}

// IssueToken creates a new session token for the given tenant, agent, and source IP.
func (sm *SessionManager) IssueToken(tenantID, agentID, sourceIP string) (string, SessionClaims, error) {
	sessionID, err := generateSessionID()
	if err != nil {
		return "", SessionClaims{}, err
	}

	now := time.Now()
	claims := SessionClaims{
		TenantID:  tenantID,
		AgentID:   agentID,
		SessionID: sessionID,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(sm.ttl).Unix(),
		SourceIP:  sourceIP,
	}

	token, err := sm.signClaims(&claims)
	if err != nil {
		return "", SessionClaims{}, err
	}

	// Store session for revocation checking.
	sm.sessions.Set(sessionID, claims)

	return token, claims, nil
}

// ValidateToken verifies a session token and returns the claims.
// It checks the signature, expiry, and source IP binding.
func (sm *SessionManager) ValidateToken(token, sourceIP string) (SessionClaims, error) {
	claims, err := sm.verifyClaims(token)
	if err != nil {
		return SessionClaims{}, err
	}

	// Check expiry first (before cache lookup, since the cache also expires).
	now := time.Now().Unix()
	if now > claims.ExpiresAt {
		return SessionClaims{}, ErrSessionExpired
	}

	// Verify source IP binding.
	if claims.SourceIP != "" && sourceIP != "" && claims.SourceIP != sourceIP {
		return SessionClaims{}, ErrSessionIPMismatch
	}

	// Verify the session hasn't been explicitly revoked.
	// Note: cache expiry is aligned with token TTL, so a missing cache entry
	// after the expiry check above means explicit revocation.
	if _, found := sm.sessions.Get(claims.SessionID); !found {
		return SessionClaims{}, ErrSessionRevoked
	}

	return claims, nil
}

// RevokeSession invalidates a session by its ID.
func (sm *SessionManager) RevokeSession(sessionID string) {
	sm.sessions.Invalidate(sessionID)
}

// RevokeAllForTenant invalidates all sessions for a given tenant.
func (sm *SessionManager) RevokeAllForTenant(tenantID string) {
	sm.sessions.InvalidatePrefix(tenantID + ":")
}

// SessionTokenFromRequest extracts a session token from the request.
// Checks X-Session-Token header first, then Authorization: Bearer if it
// looks like a session token (contains a dot separator).
func SessionTokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if token := strings.TrimSpace(r.Header.Get(SessionTokenHeader)); token != "" {
		return token
	}
	// Check Bearer token — session tokens contain a "." separator
	// while tenant keys typically don't.
	if token := bearerToken(r.Header.Get(AuthorizationHeader)); token != "" && strings.Contains(token, ".") {
		return token
	}
	return ""
}

func (sm *SessionManager) signClaims(claims *SessionClaims) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}

	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := sm.computeHMAC([]byte(encodedPayload))
	encodedSig := base64.RawURLEncoding.EncodeToString(signature)

	return encodedPayload + "." + encodedSig, nil
}

func (sm *SessionManager) verifyClaims(token string) (SessionClaims, error) {
	parts := strings.SplitN(token, ".", 2) //nolint:mnd // token format: payload.signature
	if len(parts) != 2 {                   //nolint:mnd // token format: payload.signature
		return SessionClaims{}, ErrSessionInvalid
	}

	encodedPayload := parts[0]
	encodedSig := parts[1]

	// Verify signature.
	expectedSig := sm.computeHMAC([]byte(encodedPayload))
	providedSig, err := base64.RawURLEncoding.DecodeString(encodedSig)
	if err != nil {
		return SessionClaims{}, ErrSessionInvalid
	}
	if !hmac.Equal(expectedSig, providedSig) {
		return SessionClaims{}, ErrSessionInvalid
	}

	// Decode payload.
	payload, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return SessionClaims{}, ErrSessionInvalid
	}

	var claims SessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return SessionClaims{}, ErrSessionInvalid
	}

	return claims, nil
}

func (sm *SessionManager) computeHMAC(data []byte) []byte {
	mac := hmac.New(sha256.New, sm.signingKey)
	mac.Write(data)
	return mac.Sum(nil)
}

func generateSessionID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return "sess_" + base64.RawURLEncoding.EncodeToString(buf[:]), nil
}
