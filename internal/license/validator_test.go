package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testKeypair generates a fresh Ed25519 keypair for testing.
func testKeypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return pub, priv
}

// signTestToken signs a JWT with the given claims using the private key.
func signTestToken(t *testing.T, priv ed25519.PrivateKey, claims map[string]any) []byte {
	t.Helper()

	builder := jwt.NewBuilder()
	if iss, ok := claims["iss"].(string); ok {
		builder = builder.Issuer(iss)
	}
	if sub, ok := claims["sub"].(string); ok {
		builder = builder.Subject(sub)
	}
	if iat, ok := claims["iat"].(time.Time); ok {
		builder = builder.IssuedAt(iat)
	}
	if exp, ok := claims["exp"].(time.Time); ok {
		builder = builder.Expiration(exp)
	}
	if nbf, ok := claims["nbf"].(time.Time); ok {
		builder = builder.NotBefore(nbf)
	}

	tok, err := builder.Build()
	require.NoError(t, err)

	for k, v := range claims {
		switch k {
		case "iss", "sub", "iat", "exp", "nbf":
			continue
		default:
			require.NoError(t, tok.Set(k, v))
		}
	}

	key, err := jwk.Import(priv)
	require.NoError(t, err)

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.EdDSA(), key))
	require.NoError(t, err)
	return signed
}

func validClaims(exp time.Time) map[string]any {
	return map[string]any{
		"iss":         "rbitr",
		"sub":         "Test Corp",
		"iat":         time.Now(),
		"exp":         exp,
		"key_version": float64(1),
		"tier":        "paid",
		"entitlements": map[string]any{
			"max_tenants":           float64(-1),
			"max_agents_per_tenant": float64(-1),
			"max_active_keys":       float64(-1),
			"monthly_action_limit":  float64(-1),
			"audit_retention_days":  float64(90),
			"approval_workflows":    true,
			"evidence_export":       true,
			"integrations":          true,
			"custom_policies":       true,
		},
		"licensee": map[string]any{
			"name":  "Test Corp",
			"email": "test@example.com",
		},
	}
}

func TestValidateBytes_ValidKey(t *testing.T) {
	pub, priv := testKeypair(t)
	exp := time.Now().Add(365 * 24 * time.Hour)
	token := signTestToken(t, priv, validClaims(exp))

	v := NewValidator(pub, "")
	info, err := v.ValidateBytes(token)
	require.NoError(t, err)

	assert.True(t, info.Valid)
	assert.Equal(t, "paid", info.Tier)
	assert.Equal(t, 1, info.KeyVersion)
	assert.Equal(t, "Test Corp", info.Licensee)
	assert.Equal(t, "test@example.com", info.Email)
	assert.True(t, info.Entitlements.ApprovalWorkflows)
	assert.True(t, info.Entitlements.Integrations)
	assert.Equal(t, Unlimited, info.Entitlements.MaxTenants)
}

func TestValidateBytes_ExpiredKey(t *testing.T) {
	pub, priv := testKeypair(t)
	// Expired 2 days ago (beyond 24h grace period).
	exp := time.Now().Add(-48 * time.Hour)
	token := signTestToken(t, priv, validClaims(exp))

	v := NewValidator(pub, "")
	_, err := v.ValidateBytes(token)
	assert.ErrorIs(t, err, ErrLicenseExpired)
}

func TestValidateBytes_ExpiredWithinGracePeriod(t *testing.T) {
	pub, priv := testKeypair(t)
	// Expired 12 hours ago (within 24h grace period).
	exp := time.Now().Add(-12 * time.Hour)
	claims := validClaims(exp)
	// Set nbf in the past so it doesn't interfere.
	claims["nbf"] = time.Now().Add(-365 * 24 * time.Hour)
	token := signTestToken(t, priv, claims)

	v := NewValidator(pub, "")
	info, err := v.ValidateBytes(token)
	require.NoError(t, err)
	assert.True(t, info.Valid)
}

func TestValidateBytes_TamperedKey(t *testing.T) {
	pub, _ := testKeypair(t)
	_, otherPriv := testKeypair(t)

	exp := time.Now().Add(365 * 24 * time.Hour)
	token := signTestToken(t, otherPriv, validClaims(exp))

	v := NewValidator(pub, "")
	_, err := v.ValidateBytes(token)
	assert.ErrorIs(t, err, ErrInvalidSignature)
}

func TestValidateBytes_InvalidIssuer(t *testing.T) {
	pub, priv := testKeypair(t)
	claims := validClaims(time.Now().Add(365 * 24 * time.Hour))
	claims["iss"] = "not-rbitr"
	token := signTestToken(t, priv, claims)

	v := NewValidator(pub, "")
	_, err := v.ValidateBytes(token)
	assert.ErrorIs(t, err, ErrInvalidIssuer)
}

func TestValidateBytes_KeyVersionTooOld(t *testing.T) {
	pub, priv := testKeypair(t)
	claims := validClaims(time.Now().Add(365 * 24 * time.Hour))
	claims["key_version"] = float64(0)
	token := signTestToken(t, priv, claims)

	v := NewValidator(pub, "")
	_, err := v.ValidateBytes(token)
	assert.ErrorIs(t, err, ErrKeyVersionTooOld)
}

func TestValidateBytes_KeyVersionTooNew(t *testing.T) {
	pub, priv := testKeypair(t)
	claims := validClaims(time.Now().Add(365 * 24 * time.Hour))
	claims["key_version"] = float64(999)
	token := signTestToken(t, priv, claims)

	v := NewValidator(pub, "")
	_, err := v.ValidateBytes(token)
	assert.ErrorIs(t, err, ErrKeyVersionTooNew)
}

func TestValidateBytes_MissingLicensee(t *testing.T) {
	pub, priv := testKeypair(t)
	claims := validClaims(time.Now().Add(365 * 24 * time.Hour))
	delete(claims, "licensee")
	token := signTestToken(t, priv, claims)

	v := NewValidator(pub, "")
	_, err := v.ValidateBytes(token)
	assert.ErrorIs(t, err, ErrMissingLicensee)
}

func TestValidateBytes_UnknownTier(t *testing.T) {
	pub, priv := testKeypair(t)
	claims := validClaims(time.Now().Add(365 * 24 * time.Hour))
	claims["tier"] = "enterprise"
	token := signTestToken(t, priv, claims)

	v := NewValidator(pub, "")
	_, err := v.ValidateBytes(token)
	assert.ErrorIs(t, err, ErrMalformedLicense)
}

func TestValidateBytes_MergeOverDefaults(t *testing.T) {
	pub, priv := testKeypair(t)
	claims := validClaims(time.Now().Add(365 * 24 * time.Hour))

	// Remove some entitlement fields to test merge-over-defaults.
	ent, ok := claims["entitlements"].(map[string]any)
	require.True(t, ok, "entitlements must be map[string]any")
	delete(ent, "audit_retention_days")
	token := signTestToken(t, priv, claims)

	v := NewValidator(pub, "")
	info, err := v.ValidateBytes(token)
	require.NoError(t, err)

	// Should fall back to paid default (90 days).
	assert.Equal(t, 90, info.Entitlements.AuditRetentionDays)
}

func TestLoadAndValidate_MissingFile(t *testing.T) {
	pub, _ := testKeypair(t)
	v := NewValidator(pub, "/nonexistent/license.key")
	v.LoadAndValidate()

	info := v.Info()
	assert.False(t, info.Valid)
	assert.Equal(t, "free", info.Tier)
	assert.Equal(t, FreeTierDefaults(), info.Entitlements)
}

func TestLoadAndValidate_ValidFile(t *testing.T) {
	pub, priv := testKeypair(t)
	exp := time.Now().Add(365 * 24 * time.Hour)
	token := signTestToken(t, priv, validClaims(exp))

	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "license.key")
	require.NoError(t, os.WriteFile(keyPath, token, 0o600))

	v := NewValidator(pub, keyPath)
	v.LoadAndValidate()

	info := v.Info()
	assert.True(t, info.Valid)
	assert.Equal(t, "paid", info.Tier)
}

func TestLoadAndValidate_InvalidFile(t *testing.T) {
	pub, _ := testKeypair(t)

	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "license.key")
	require.NoError(t, os.WriteFile(keyPath, []byte("garbage-data"), 0o600))

	v := NewValidator(pub, keyPath)
	v.LoadAndValidate()

	info := v.Info()
	assert.False(t, info.Valid)
	assert.Equal(t, "free", info.Tier)
}

func TestFingerprint(t *testing.T) {
	data := []byte("test-license-data")
	fp := Fingerprint(data)
	assert.Len(t, fp, 64) // SHA-256 hex = 64 chars
	// Deterministic.
	assert.Equal(t, fp, Fingerprint(data))
}

func TestEntitlements_ThreadSafe(t *testing.T) {
	pub, priv := testKeypair(t)
	exp := time.Now().Add(365 * 24 * time.Hour)
	token := signTestToken(t, priv, validClaims(exp))

	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "license.key")
	require.NoError(t, os.WriteFile(keyPath, token, 0o600))

	v := NewValidator(pub, keyPath)
	v.LoadAndValidate()

	// Concurrent reads should not panic.
	done := make(chan struct{})
	for range 10 {
		go func() {
			_ = v.Entitlements()
			_ = v.Info()
			done <- struct{}{}
		}()
	}
	for range 10 {
		<-done
	}
}

func TestValidateBytes_FreeTierKey(t *testing.T) {
	pub, priv := testKeypair(t)
	exp := time.Now().Add(365 * 24 * time.Hour)
	claims := map[string]any{
		"iss":         "rbitr",
		"sub":         "Free User",
		"iat":         time.Now(),
		"exp":         exp,
		"key_version": float64(1),
		"tier":        "free",
		"entitlements": map[string]any{
			"max_tenants":           float64(1),
			"max_agents_per_tenant": float64(1),
			"max_active_keys":       float64(1),
			"monthly_action_limit":  float64(10000),
			"audit_retention_days":  float64(7),
			"approval_workflows":    false,
			"evidence_export":       false,
			"integrations":          false,
			"custom_policies":       false,
		},
		"licensee": map[string]any{
			"name":  "Free User",
			"email": "free@example.com",
		},
	}
	token := signTestToken(t, priv, claims)

	v := NewValidator(pub, "")
	info, err := v.ValidateBytes(token)
	require.NoError(t, err)

	assert.True(t, info.Valid)
	assert.Equal(t, "free", info.Tier)
	assert.Equal(t, 1, info.Entitlements.MaxTenants)
	assert.False(t, info.Entitlements.ApprovalWorkflows)
}

// Ensure json.RawMessage extraction works in validateFlatClaims.
func TestValidateBytes_EntitlementsAsRawJSON(t *testing.T) {
	pub, priv := testKeypair(t)
	exp := time.Now().Add(365 * 24 * time.Hour)

	// Build entitlements as raw JSON to verify the Get() -> json.RawMessage path.
	entMap := map[string]any{
		"max_tenants":           -1,
		"max_agents_per_tenant": -1,
		"max_active_keys":       -1,
		"monthly_action_limit":  -1,
		"audit_retention_days":  180,
		"approval_workflows":    true,
		"evidence_export":       true,
		"integrations":          true,
		"custom_policies":       true,
	}
	entBytes, _ := json.Marshal(entMap)
	var entRaw json.RawMessage
	_ = json.Unmarshal(entBytes, &entRaw)

	claims := map[string]any{
		"iss":          "rbitr",
		"sub":          "Test Corp",
		"iat":          time.Now(),
		"exp":          exp,
		"key_version":  float64(1),
		"tier":         "paid",
		"entitlements": entMap,
		"licensee": map[string]any{
			"name":  "Test Corp",
			"email": "test@example.com",
		},
	}
	token := signTestToken(t, priv, claims)

	v := NewValidator(pub, "")
	info, err := v.ValidateBytes(token)
	require.NoError(t, err)
	assert.Equal(t, 180, info.Entitlements.AuditRetentionDays)
}
