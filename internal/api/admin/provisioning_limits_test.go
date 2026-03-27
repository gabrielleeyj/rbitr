package admin

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/license"
	"github.com/gabrielleeyj/rbitr/internal/store"
)

func testLicenseProvider(t *testing.T, tier string, maxTenants, maxActiveKeys int) *license.SelfManagedProvider {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	now := time.Now()
	exp := now.Add(365 * 24 * time.Hour)

	ent := license.DefaultsForTier(tier)
	ent.MaxTenants = maxTenants
	ent.MaxActiveKeys = maxActiveKeys

	tok, err := jwt.NewBuilder().
		Issuer("rbitr").
		Subject("Test").
		IssuedAt(now).
		NotBefore(now).
		Expiration(exp).
		Build()
	require.NoError(t, err)

	require.NoError(t, tok.Set("key_version", float64(1)))
	require.NoError(t, tok.Set("tier", tier))
	require.NoError(t, tok.Set("entitlements", ent))
	require.NoError(t, tok.Set("licensee", map[string]string{
		"name":  "Test",
		"email": "test@example.com",
	}))

	key, err := jwk.Import(priv)
	require.NoError(t, err)
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.EdDSA(), key))
	require.NoError(t, err)

	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "license.key")
	require.NoError(t, os.WriteFile(keyPath, signed, 0o600))

	v := license.NewValidator(pub, keyPath)
	v.LoadAndValidate()
	w := license.NewWatcher(v, keyPath)
	return license.NewSelfManagedProvider(v, w)
}

// --- checkTenantLimit ---

func TestCheckTenantLimit_NilValidator(t *testing.T) {
	deps := &Dependencies{}
	violation, err := deps.checkTenantLimit(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, violation)
}

func TestCheckTenantLimit_Unlimited(t *testing.T) {
	v := testLicenseProvider(t, "paid", license.Unlimited, license.Unlimited)
	storeMock := store.NewMockStoreAPI(t)
	// Store should NOT be called for unlimited tier.
	deps := &Dependencies{
		LicenseProvider: v,
		Store:           storeMock,
	}

	violation, err := deps.checkTenantLimit(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, violation)
}

func TestCheckTenantLimit_UnderLimit(t *testing.T) {
	v := testLicenseProvider(t, "free", 1, 1)
	storeMock := store.NewMockStoreAPI(t)
	storeMock.EXPECT().CountTenants(mock.Anything).Return(0, nil)

	deps := &Dependencies{
		LicenseProvider: v,
		Store:           storeMock,
	}

	violation, err := deps.checkTenantLimit(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, violation)
}

func TestCheckTenantLimit_AtLimit(t *testing.T) {
	v := testLicenseProvider(t, "free", 1, 1)
	storeMock := store.NewMockStoreAPI(t)
	storeMock.EXPECT().CountTenants(mock.Anything).Return(1, nil)

	deps := &Dependencies{
		LicenseProvider: v,
		Store:           storeMock,
	}

	violation, err := deps.checkTenantLimit(context.Background())
	assert.NoError(t, err)
	require.NotNil(t, violation)
	assert.Equal(t, "tenants", violation.Resource)
	assert.Equal(t, 1, violation.Current)
	assert.Equal(t, 1, violation.Limit)
	assert.Contains(t, violation.Message, "license key")
}

func TestCheckTenantLimit_StoreError(t *testing.T) {
	v := testLicenseProvider(t, "free", 1, 1)
	storeMock := store.NewMockStoreAPI(t)
	storeMock.EXPECT().CountTenants(mock.Anything).Return(0, errors.New("db error"))

	deps := &Dependencies{
		LicenseProvider: v,
		Store:           storeMock,
	}

	violation, err := deps.checkTenantLimit(context.Background())
	assert.Error(t, err)
	assert.Nil(t, violation)
}

// --- checkActiveKeyLimit ---

func TestCheckActiveKeyLimit_NilValidator(t *testing.T) {
	deps := &Dependencies{}
	violation, err := deps.checkActiveKeyLimit(context.Background(), "tenant-1")
	assert.NoError(t, err)
	assert.Nil(t, violation)
}

func TestCheckActiveKeyLimit_Unlimited(t *testing.T) {
	v := testLicenseProvider(t, "paid", license.Unlimited, license.Unlimited)
	storeMock := store.NewMockStoreAPI(t)
	deps := &Dependencies{
		LicenseProvider: v,
		Store:           storeMock,
	}

	violation, err := deps.checkActiveKeyLimit(context.Background(), "tenant-1")
	assert.NoError(t, err)
	assert.Nil(t, violation)
}

func TestCheckActiveKeyLimit_UnderLimit(t *testing.T) {
	v := testLicenseProvider(t, "free", 1, 1)
	storeMock := store.NewMockStoreAPI(t)
	storeMock.EXPECT().CountActiveKeysByTenant(mock.Anything, "tenant-1").Return(0, nil)

	deps := &Dependencies{
		LicenseProvider: v,
		Store:           storeMock,
	}

	violation, err := deps.checkActiveKeyLimit(context.Background(), "tenant-1")
	assert.NoError(t, err)
	assert.Nil(t, violation)
}

func TestCheckActiveKeyLimit_AtLimit(t *testing.T) {
	v := testLicenseProvider(t, "free", 1, 1)
	storeMock := store.NewMockStoreAPI(t)
	storeMock.EXPECT().CountActiveKeysByTenant(mock.Anything, "tenant-1").Return(1, nil)

	deps := &Dependencies{
		LicenseProvider: v,
		Store:           storeMock,
	}

	violation, err := deps.checkActiveKeyLimit(context.Background(), "tenant-1")
	assert.NoError(t, err)
	require.NotNil(t, violation)
	assert.Equal(t, "active_keys", violation.Resource)
	assert.Equal(t, 1, violation.Current)
	assert.Equal(t, 1, violation.Limit)
	assert.Contains(t, violation.Message, "Revoke")
}

func TestCheckActiveKeyLimit_StoreError(t *testing.T) {
	v := testLicenseProvider(t, "free", 1, 1)
	storeMock := store.NewMockStoreAPI(t)
	storeMock.EXPECT().CountActiveKeysByTenant(mock.Anything, "tenant-1").Return(0, errors.New("db error"))

	deps := &Dependencies{
		LicenseProvider: v,
		Store:           storeMock,
	}

	violation, err := deps.checkActiveKeyLimit(context.Background(), "tenant-1")
	assert.Error(t, err)
	assert.Nil(t, violation)
}
