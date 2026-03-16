package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSessionManagerIssueAndValidate(t *testing.T) {
	t.Parallel()

	sm := newSessionManagerWithKey([]byte("test-signing-key-32-bytes-long!!"), 15*time.Minute)

	token, claims, err := sm.IssueToken("tenant_1", "agent_bot", "10.0.1.42")
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.Equal(t, "tenant_1", claims.TenantID)
	require.Equal(t, "agent_bot", claims.AgentID)
	require.Equal(t, "10.0.1.42", claims.SourceIP)
	require.Contains(t, claims.SessionID, "sess_")

	// Validate the token.
	validated, err := sm.ValidateToken(token, "10.0.1.42")
	require.NoError(t, err)
	require.Equal(t, claims.TenantID, validated.TenantID)
	require.Equal(t, claims.AgentID, validated.AgentID)
	require.Equal(t, claims.SessionID, validated.SessionID)
}

func TestSessionManagerExpired(t *testing.T) {
	t.Parallel()

	sm := newSessionManagerWithKey([]byte("test-signing-key-32-bytes-long!!"), 1*time.Second)

	token, _, err := sm.IssueToken("tenant_1", "agent_bot", "10.0.1.42")
	require.NoError(t, err)

	// Wait for expiry. Token uses Unix seconds, so we need to cross
	// a full second boundary to guarantee now > expiresAt.
	time.Sleep(2100 * time.Millisecond) // deliberately waiting past second boundary

	_, err = sm.ValidateToken(token, "10.0.1.42")
	require.ErrorIs(t, err, ErrSessionExpired)
}

func TestSessionManagerIPMismatch(t *testing.T) {
	t.Parallel()

	sm := newSessionManagerWithKey([]byte("test-signing-key-32-bytes-long!!"), 15*time.Minute)

	token, _, err := sm.IssueToken("tenant_1", "agent_bot", "10.0.1.42")
	require.NoError(t, err)

	_, err = sm.ValidateToken(token, "192.168.1.1")
	require.ErrorIs(t, err, ErrSessionIPMismatch)
}

func TestSessionManagerTamperedToken(t *testing.T) {
	t.Parallel()

	sm := newSessionManagerWithKey([]byte("test-signing-key-32-bytes-long!!"), 15*time.Minute)

	token, _, err := sm.IssueToken("tenant_1", "agent_bot", "10.0.1.42")
	require.NoError(t, err)

	// Tamper with the token.
	tampered := token + "x"
	_, err = sm.ValidateToken(tampered, "10.0.1.42")
	require.ErrorIs(t, err, ErrSessionInvalid)
}

func TestSessionManagerDifferentKey(t *testing.T) {
	t.Parallel()

	sm1 := newSessionManagerWithKey([]byte("test-signing-key-32-bytes-long!!"), 15*time.Minute)
	sm2 := newSessionManagerWithKey([]byte("different-signing-key-32-bytes!!"), 15*time.Minute)

	token, _, err := sm1.IssueToken("tenant_1", "agent_bot", "10.0.1.42")
	require.NoError(t, err)

	// Token from sm1 should not validate with sm2.
	_, err = sm2.ValidateToken(token, "10.0.1.42")
	require.ErrorIs(t, err, ErrSessionInvalid)
}

func TestSessionManagerRevoke(t *testing.T) {
	t.Parallel()

	sm := newSessionManagerWithKey([]byte("test-signing-key-32-bytes-long!!"), 15*time.Minute)

	token, claims, err := sm.IssueToken("tenant_1", "agent_bot", "10.0.1.42")
	require.NoError(t, err)

	// Validate before revocation.
	_, err = sm.ValidateToken(token, "10.0.1.42")
	require.NoError(t, err)

	// Revoke.
	sm.RevokeSession(claims.SessionID)

	// Should fail after revocation.
	_, err = sm.ValidateToken(token, "10.0.1.42")
	require.ErrorIs(t, err, ErrSessionRevoked)
}

func TestSessionManagerEmptySourceIP(t *testing.T) {
	t.Parallel()

	sm := newSessionManagerWithKey([]byte("test-signing-key-32-bytes-long!!"), 15*time.Minute)

	// Issue with empty IP — should not enforce IP binding.
	token, _, err := sm.IssueToken("tenant_1", "agent_bot", "")
	require.NoError(t, err)

	_, err = sm.ValidateToken(token, "10.0.1.42")
	require.NoError(t, err)
}

func TestSessionManagerInvalidTokenFormat(t *testing.T) {
	t.Parallel()

	sm := newSessionManagerWithKey([]byte("test-signing-key-32-bytes-long!!"), 15*time.Minute)

	cases := []string{
		"",
		"no-dot-here",
		"too.many.dots",
		".empty-payload",
		"empty-sig.",
	}

	for _, token := range cases {
		_, err := sm.ValidateToken(token, "10.0.1.42")
		require.ErrorIs(t, err, ErrSessionInvalid, "token: %q", token)
	}
}

func TestNewSessionManager(t *testing.T) {
	t.Parallel()

	sm, err := NewSessionManager(15 * time.Minute)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Len(t, sm.signingKey, sessionSigningKeyLen)
}

func TestSessionTokenFromRequest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		headers  map[string]string
		expected string
	}{
		{
			name:     "X-Session-Token header",
			headers:  map[string]string{SessionTokenHeader: "payload.signature"},
			expected: "payload.signature",
		},
		{
			name:     "Bearer with dot (session token)",
			headers:  map[string]string{AuthorizationHeader: "Bearer payload.signature"},
			expected: "payload.signature",
		},
		{
			name:     "Bearer without dot (tenant key, not session)",
			headers:  map[string]string{AuthorizationHeader: "Bearer rbtr_key_abc123"},
			expected: "",
		},
		{
			name:     "no headers",
			headers:  map[string]string{},
			expected: "",
		},
		{
			name:     "X-Session-Token takes precedence over Bearer",
			headers:  map[string]string{SessionTokenHeader: "sess.token", AuthorizationHeader: "Bearer other.token"},
			expected: "sess.token",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "/", http.NoBody)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			got := SessionTokenFromRequest(req)
			require.Equal(t, tc.expected, got)
		})
	}
}

func TestSessionTokenFromRequestNil(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", SessionTokenFromRequest(nil))
}
