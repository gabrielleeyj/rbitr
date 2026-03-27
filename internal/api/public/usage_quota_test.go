package public

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

func testLicenseProvider(t *testing.T, tier string, monthlyLimit int64) *license.SelfManagedProvider {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	exp := time.Now().Add(365 * 24 * time.Hour)
	now := time.Now()

	ent := license.DefaultsForTier(tier)
	ent.MonthlyActionLimit = monthlyLimit

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

func TestEnforceUsageQuota_NilValidator(t *testing.T) {
	deps := &Dependencies{}
	violation, err := deps.enforceUsageQuota(context.Background(), "tenant-1")
	assert.NoError(t, err)
	assert.Nil(t, violation)
}

func TestEnforceUsageQuota_UnlimitedSkipsMetering(t *testing.T) {
	v := testLicenseProvider(t, "paid", int64(license.Unlimited))
	storeMock := store.NewMockStoreAPI(t)
	// Store should NOT be called for unlimited tier.
	deps := &Dependencies{
		LicenseProvider: v,
		Store:           storeMock,
	}

	violation, err := deps.enforceUsageQuota(context.Background(), "tenant-1")
	assert.NoError(t, err)
	assert.Nil(t, violation)
	// Mock will fail if any unexpected calls were made.
}

func TestEnforceUsageQuota_UnderLimit(t *testing.T) {
	v := testLicenseProvider(t, "free", 10_000)
	storeMock := store.NewMockStoreAPI(t)
	storeMock.EXPECT().IncrementUsageMeter(
		mock.Anything,
		"tenant-1",
		currentPeriod(),
	).Return(int64(5000), nil)

	deps := &Dependencies{
		LicenseProvider: v,
		Store:           storeMock,
	}

	violation, err := deps.enforceUsageQuota(context.Background(), "tenant-1")
	assert.NoError(t, err)
	assert.Nil(t, violation)
}

func TestEnforceUsageQuota_AtLimit(t *testing.T) {
	v := testLicenseProvider(t, "free", 10_000)
	storeMock := store.NewMockStoreAPI(t)
	storeMock.EXPECT().IncrementUsageMeter(
		mock.Anything,
		"tenant-1",
		currentPeriod(),
	).Return(int64(10_000), nil)

	deps := &Dependencies{
		LicenseProvider: v,
		Store:           storeMock,
	}

	// At exactly the limit is still allowed (count == limit, not count > limit).
	violation, err := deps.enforceUsageQuota(context.Background(), "tenant-1")
	assert.NoError(t, err)
	assert.Nil(t, violation)
}

func TestEnforceUsageQuota_OverLimit(t *testing.T) {
	v := testLicenseProvider(t, "free", 10_000)
	storeMock := store.NewMockStoreAPI(t)
	storeMock.EXPECT().IncrementUsageMeter(
		mock.Anything,
		"tenant-1",
		currentPeriod(),
	).Return(int64(10_001), nil)

	deps := &Dependencies{
		LicenseProvider: v,
		Store:           storeMock,
	}

	violation, err := deps.enforceUsageQuota(context.Background(), "tenant-1")
	assert.NoError(t, err)
	require.NotNil(t, violation)
	assert.Equal(t, int64(10_000), violation.Limit)
	assert.Equal(t, int64(10_001), violation.Used)
	assert.Equal(t, currentPeriod(), violation.Period)
	assert.Contains(t, violation.Message, "license key")
}

func TestEnforceUsageQuota_StoreError(t *testing.T) {
	v := testLicenseProvider(t, "free", 10_000)
	storeMock := store.NewMockStoreAPI(t)
	storeMock.EXPECT().IncrementUsageMeter(
		mock.Anything,
		"tenant-1",
		currentPeriod(),
	).Return(int64(0), errors.New("db connection failed"))

	deps := &Dependencies{
		LicenseProvider: v,
		Store:           storeMock,
	}

	violation, err := deps.enforceUsageQuota(context.Background(), "tenant-1")
	assert.Error(t, err)
	assert.Nil(t, violation)
}

func TestEnforceUsageQuota_NilStore(t *testing.T) {
	v := testLicenseProvider(t, "free", 10_000)
	deps := &Dependencies{
		LicenseProvider: v,
		Store:           nil,
	}

	violation, err := deps.enforceUsageQuota(context.Background(), "tenant-1")
	assert.NoError(t, err)
	assert.Nil(t, violation)
}

func TestCurrentPeriod(t *testing.T) {
	period := currentPeriod()
	assert.Regexp(t, `^\d{4}-\d{2}$`, period)
}
