package license_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"

	"github.com/gabrielleeyj/rbitr/internal/license"
)

func TestSelfManaged_NoKeyFile_FreeTier(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	cfg := &license.ProviderConfig{
		Name:    license.ProviderSelfManaged,
		PubKey:  pub,
		KeyPath: "/tmp/nonexistent-rbitr-key-" + t.Name(),
	}

	provider, reporter, err := license.NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	// Should fall back to free tier.
	info := provider.Info()
	if info.Valid {
		t.Error("expected Valid = false with no key file")
	}
	if info.Tier != "free" {
		t.Errorf("Tier = %q, want %q", info.Tier, "free")
	}

	ent := provider.Entitlements()
	if ent.MaxTenants != 1 {
		t.Errorf("MaxTenants = %d, want 1 (free tier)", ent.MaxTenants)
	}
	if ent.MonthlyActionLimit != 10_000 {
		t.Errorf("MonthlyActionLimit = %d, want 10000 (free tier)", ent.MonthlyActionLimit)
	}
	if ent.ApprovalWorkflows {
		t.Error("ApprovalWorkflows should be false for free tier")
	}

	// Reporter should be a NoopReporter.
	if reporter == nil {
		t.Fatal("reporter should not be nil")
	}

	// Start/stop should work cleanly.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { provider.Start(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("provider.Start did not return")
	}
}

func TestSelfManaged_ValidKeyFile_PaidTier(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	// Create a valid JWT license key.
	token := buildTestLicenseJWT(t, priv, "paid", 1)

	// Write to a temp file.
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "license.key")
	if writeErr := os.WriteFile(keyPath, token, 0o600); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}

	cfg := &license.ProviderConfig{
		Name:    license.ProviderSelfManaged,
		PubKey:  pub,
		KeyPath: keyPath,
	}

	provider, _, err := license.NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	info := provider.Info()
	if !info.Valid {
		t.Error("expected Valid = true with valid key file")
	}
	if info.Tier != "paid" {
		t.Errorf("Tier = %q, want %q", info.Tier, "paid")
	}
	if info.Licensee != "Test Corp" {
		t.Errorf("Licensee = %q, want %q", info.Licensee, "Test Corp")
	}

	ent := provider.Entitlements()
	if !license.IsUnlimited(ent.MaxTenants) {
		t.Errorf("MaxTenants = %d, want unlimited (-1)", ent.MaxTenants)
	}
	if !ent.ApprovalWorkflows {
		t.Error("ApprovalWorkflows should be true for paid tier")
	}
}

func TestSelfManaged_ExpiredKey_FreeTier(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	// Create an expired JWT license key.
	token := buildExpiredLicenseJWT(t, priv)

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "license.key")
	if writeErr := os.WriteFile(keyPath, token, 0o600); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}

	cfg := &license.ProviderConfig{
		Name:    license.ProviderSelfManaged,
		PubKey:  pub,
		KeyPath: keyPath,
	}

	provider, _, err := license.NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	info := provider.Info()
	if info.Valid {
		t.Error("expected Valid = false with expired key")
	}
	if info.Tier != "free" {
		t.Errorf("Tier = %q, want %q", info.Tier, "free")
	}
}

func TestSelfManaged_KeyFileUpdate_RefreshesEntitlements(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "license.key")

	// Start without a key file → free tier.
	cfg := &license.ProviderConfig{
		Name:    license.ProviderSelfManaged,
		PubKey:  pub,
		KeyPath: keyPath,
	}

	provider, _, err := license.NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	if provider.Info().Valid {
		t.Fatal("expected Valid = false initially")
	}

	// Write a valid key file and trigger reload via SelfManagedManager.
	token := buildTestLicenseJWT(t, priv, "paid", 1)
	if err := os.WriteFile(keyPath, token, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	mgr, ok := provider.(license.SelfManagedManager)
	if !ok {
		t.Fatal("provider should implement SelfManagedManager")
	}

	mgr.LoadAndValidate()

	if !provider.Info().Valid {
		t.Error("expected Valid = true after key file write + reload")
	}
	if provider.Info().Tier != "paid" {
		t.Errorf("Tier = %q, want %q after reload", provider.Info().Tier, "paid")
	}
}

func TestSelfManaged_FactoryRejectsMarketplaceProviders(t *testing.T) {
	providers := []string{
		license.ProviderAWSMarketplace,
		license.ProviderGCPMarketplace,
		license.ProviderAzureMarketplace,
	}

	for _, name := range providers {
		cfg := &license.ProviderConfig{Name: name}
		_, _, err := license.NewProvider(cfg)
		if err == nil {
			t.Errorf("expected error for provider %q, got nil", name)
		}
	}
}

func TestSelfManaged_FactoryRejectsUnknownProvider(t *testing.T) {
	cfg := &license.ProviderConfig{Name: "unknown-provider"}
	_, _, err := license.NewProvider(cfg)
	if err == nil {
		t.Error("expected error for unknown provider, got nil")
	}
}

// buildTestLicenseJWT creates a signed JWT license key for testing.
func buildTestLicenseJWT(t *testing.T, priv ed25519.PrivateKey, tier string, keyVersion int) []byte {
	t.Helper()

	key, err := jwk.Import(priv)
	if err != nil {
		t.Fatalf("jwk.Import: %v", err)
	}

	now := time.Now()
	tok, err := jwt.NewBuilder().
		Issuer("rbitr").
		Subject("Test Corp").
		IssuedAt(now).
		Expiration(now.Add(365*24*time.Hour)).
		Claim("key_version", keyVersion).
		Claim("tier", tier).
		Claim("licensee", map[string]string{
			"name":  "Test Corp",
			"email": "admin@testcorp.com",
		}).
		Claim("entitlements", map[string]any{
			"max_tenants":           -1,
			"max_agents_per_tenant": -1,
			"max_active_keys":       -1,
			"monthly_action_limit":  -1,
			"audit_retention_days":  -1,
			"approval_workflows":    true,
			"evidence_export":       true,
			"integrations":          true,
			"custom_policies":       true,
		}).
		Build()
	if err != nil {
		t.Fatalf("jwt.NewBuilder: %v", err)
	}

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.EdDSA(), key))
	if err != nil {
		t.Fatalf("jwt.Sign: %v", err)
	}

	return signed
}

// buildExpiredLicenseJWT creates a signed JWT that is already expired (beyond the grace period).
func buildExpiredLicenseJWT(t *testing.T, priv ed25519.PrivateKey) []byte {
	t.Helper()

	key, err := jwk.Import(priv)
	if err != nil {
		t.Fatalf("jwk.Import: %v", err)
	}

	past := time.Now().Add(-48 * time.Hour) // Beyond the 24h grace period
	tok, err := jwt.NewBuilder().
		Issuer("rbitr").
		Subject("Test Corp").
		IssuedAt(past.Add(-365*24*time.Hour)).
		Expiration(past).
		Claim("key_version", 1).
		Claim("tier", "paid").
		Claim("licensee", map[string]string{
			"name":  "Test Corp",
			"email": "admin@testcorp.com",
		}).
		Claim("entitlements", map[string]any{
			"max_tenants": -1,
		}).
		Build()
	if err != nil {
		t.Fatalf("jwt.NewBuilder: %v", err)
	}

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.EdDSA(), key))
	if err != nil {
		t.Fatalf("jwt.Sign: %v", err)
	}

	return signed
}
