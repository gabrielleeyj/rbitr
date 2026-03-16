package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvenanceManager_IssueAndValidate(t *testing.T) {
	pm, err := NewProvenanceManager(5)
	require.NoError(t, err)

	token, err := pm.IssueToken("tenant_a", "dec_123", 0)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	claims, err := pm.ValidateToken(token)
	require.NoError(t, err)
	assert.Equal(t, "tenant_a", claims.SourceTenantID)
	assert.Equal(t, "dec_123", claims.SourceDecisionID)
	assert.Equal(t, 1, claims.ChainDepth)
}

func TestProvenanceManager_ChainDepthIncrement(t *testing.T) {
	pm, err := NewProvenanceManager(5)
	require.NoError(t, err)

	token, err := pm.IssueToken("t1", "d1", 2)
	require.NoError(t, err)

	claims, err := pm.ValidateToken(token)
	require.NoError(t, err)
	assert.Equal(t, 3, claims.ChainDepth)
}

func TestProvenanceManager_ChainDepthExceeded(t *testing.T) {
	pm, err := NewProvenanceManager(3)
	require.NoError(t, err)

	_, err = pm.IssueToken("t1", "d1", 3)
	require.ErrorIs(t, err, ErrChainDepthExceeded)
}

func TestProvenanceManager_ExpiredToken(t *testing.T) {
	key := make([]byte, provenanceSigningKeyLen) //nolint:makezero // zero-value key is fine for testing
	pm := newProvenanceManagerWithKey(key, 5)

	// Manually create an expired token.
	claims := ProvenanceClaims{
		SourceTenantID:   "t1",
		SourceDecisionID: "d1",
		ChainDepth:       1,
		IssuedAt:         time.Now().Add(-time.Minute).Unix(),
		ExpiresAt:        time.Now().Add(-time.Second).Unix(),
	}
	token, err := pm.signClaims(&claims)
	require.NoError(t, err)

	_, err = pm.ValidateToken(token)
	require.ErrorIs(t, err, ErrProvenanceExpired)
}

func TestProvenanceManager_InvalidSignature(t *testing.T) {
	pm1, err := NewProvenanceManager(5)
	require.NoError(t, err)

	pm2, err := NewProvenanceManager(5)
	require.NoError(t, err)

	token, err := pm1.IssueToken("t1", "d1", 0)
	require.NoError(t, err)

	// Validate with a different key should fail.
	_, err = pm2.ValidateToken(token)
	require.ErrorIs(t, err, ErrProvenanceInvalid)
}

func TestProvenanceManager_MalformedToken(t *testing.T) {
	pm, err := NewProvenanceManager(5)
	require.NoError(t, err)

	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"no_dot", "abcdef"},
		{"bad_payload", "!!!.abc"},
		{"bad_sig", "eyJzcmMiOiJ0MSJ9.!!!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pm.ValidateToken(tt.token)
			require.ErrorIs(t, err, ErrProvenanceInvalid)
		})
	}
}

func TestProvenanceFromRequest(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "/", http.NoBody)
		req.Header.Set(ProvenanceHeader, "some-token")
		assert.Equal(t, "some-token", ProvenanceFromRequest(req))
	})

	t.Run("missing", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "/", http.NoBody)
		assert.Empty(t, ProvenanceFromRequest(req))
	})

	t.Run("nil_request", func(t *testing.T) {
		assert.Empty(t, ProvenanceFromRequest(nil))
	})
}

func TestProvenanceManager_ValidateChainDepthOnValidation(t *testing.T) {
	key := make([]byte, provenanceSigningKeyLen) //nolint:makezero // zero-value key is fine for testing
	pm := newProvenanceManagerWithKey(key, 2)

	// Create a token with depth exceeding max.
	claims := ProvenanceClaims{
		SourceTenantID:   "t1",
		SourceDecisionID: "d1",
		ChainDepth:       3,
		IssuedAt:         time.Now().Unix(),
		ExpiresAt:        time.Now().Add(time.Minute).Unix(),
	}
	token, err := pm.signClaims(&claims)
	require.NoError(t, err)

	_, err = pm.ValidateToken(token)
	require.ErrorIs(t, err, ErrChainDepthExceeded)
}
