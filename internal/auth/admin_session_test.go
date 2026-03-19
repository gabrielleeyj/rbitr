package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testSigningKey() []byte {
	return []byte("test-signing-key-32-bytes-long!!")
}

func TestNewAdminSessionManager(t *testing.T) {
	t.Parallel()

	mgr, err := NewAdminSessionManager(AdminSessionTTL)
	require.NoError(t, err)
	require.NotNil(t, mgr)
	require.Len(t, mgr.signingKey, adminSessionSigningKeyLen)
	require.Equal(t, AdminSessionTTL, mgr.ttl)
}

func TestAdminSessionIssueAndValidate(t *testing.T) {
	t.Parallel()

	mgr := newAdminSessionManagerWithKey(testSigningKey(), 15*time.Minute)

	user := OIDCUserInfo{
		Subject: "sub_123",
		Email:   "alice@example.com",
		Name:    "Alice",
	}
	scopes := []string{"admin:read", "admin:write"}

	token, claims, err := mgr.IssueSession(user, scopes)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.Equal(t, "alice@example.com", claims.Email)
	require.Equal(t, "Alice", claims.Name)
	require.Equal(t, "sub_123", claims.Subject)
	require.Equal(t, scopes, claims.Scopes)
	require.Equal(t, "oidc", claims.AuthType)
	require.Contains(t, claims.SessionID, "rbas_")

	validated, err := mgr.ValidateSession(token)
	require.NoError(t, err)
	require.Equal(t, claims.SessionID, validated.SessionID)
	require.Equal(t, claims.Email, validated.Email)
}

func TestAdminSessionValidateInvalidToken(t *testing.T) {
	t.Parallel()

	mgr := newAdminSessionManagerWithKey(testSigningKey(), 15*time.Minute)

	_, err := mgr.ValidateSession("tampered.token")
	require.ErrorIs(t, err, ErrAdminSessionInvalid)
}

func TestAdminSessionValidateTamperedToken(t *testing.T) {
	t.Parallel()

	mgr := newAdminSessionManagerWithKey(testSigningKey(), 15*time.Minute)

	user := OIDCUserInfo{Email: "alice@example.com", Name: "Alice", Subject: "sub_123"}
	token, _, err := mgr.IssueSession(user, []string{"admin:read"})
	require.NoError(t, err)

	tampered := token + "x"
	_, err = mgr.ValidateSession(tampered)
	require.ErrorIs(t, err, ErrAdminSessionInvalid)
}

func TestAdminSessionValidateExpired(t *testing.T) {
	t.Parallel()

	mgr := newAdminSessionManagerWithKey(testSigningKey(), 1*time.Second)

	user := OIDCUserInfo{Email: "alice@example.com", Name: "Alice", Subject: "sub_123"}
	token, _, err := mgr.IssueSession(user, []string{"admin:read"})
	require.NoError(t, err)

	// Wait for expiry. Token uses Unix seconds, so we need to cross
	// a full second boundary to guarantee now > expiresAt.
	time.Sleep(2100 * time.Millisecond)

	_, err = mgr.ValidateSession(token)
	require.ErrorIs(t, err, ErrAdminSessionExpired)
}

func TestAdminSessionRevoke(t *testing.T) {
	t.Parallel()

	mgr := newAdminSessionManagerWithKey(testSigningKey(), 15*time.Minute)

	user := OIDCUserInfo{Email: "alice@example.com", Name: "Alice", Subject: "sub_123"}
	token, claims, err := mgr.IssueSession(user, []string{"admin:read"})
	require.NoError(t, err)

	// Validate before revocation.
	_, err = mgr.ValidateSession(token)
	require.NoError(t, err)

	// Revoke.
	mgr.RevokeSession(claims.SessionID)

	// Should fail after revocation.
	_, err = mgr.ValidateSession(token)
	require.ErrorIs(t, err, ErrAdminSessionRevoked)
}

func TestIsAdminSessionToken(t *testing.T) {
	t.Parallel()

	mgr := newAdminSessionManagerWithKey(testSigningKey(), 15*time.Minute)
	user := OIDCUserInfo{Email: "alice@example.com", Name: "Alice", Subject: "sub_123"}
	token, _, err := mgr.IssueSession(user, []string{"admin:read"})
	require.NoError(t, err)

	cases := []struct {
		name     string
		token    string
		expected bool
	}{
		{name: "valid admin session token", token: token, expected: true},
		{name: "plain API key", token: "not-a-session-token", expected: false},
		{name: "empty string", token: "", expected: false},
		{name: "random string no dot", token: "nodothere", expected: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, IsAdminSessionToken(tc.token))
		})
	}
}

func TestGenerateAdminSessionID(t *testing.T) {
	t.Parallel()

	id, err := generateAdminSessionID()
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(id, "rbas_"), "session ID should start with rbas_ prefix, got: %s", id)

	// Generate a second ID and verify uniqueness.
	id2, err := generateAdminSessionID()
	require.NoError(t, err)
	require.NotEqual(t, id, id2)
}

func TestAdminSessionDifferentKey(t *testing.T) {
	t.Parallel()

	mgr1 := newAdminSessionManagerWithKey(testSigningKey(), 15*time.Minute)
	mgr2 := newAdminSessionManagerWithKey([]byte("different-signing-key-32-bytes!!"), 15*time.Minute)

	user := OIDCUserInfo{Email: "alice@example.com", Name: "Alice", Subject: "sub_123"}
	token, _, err := mgr1.IssueSession(user, []string{"admin:read"})
	require.NoError(t, err)

	// Token from mgr1 should not validate with mgr2.
	_, err = mgr2.ValidateSession(token)
	require.ErrorIs(t, err, ErrAdminSessionInvalid)
}
