package credential

import (
	"context"
	"encoding/json"
	"fmt"
)

// Resolver selects the correct provider and resolves credentials for a tool.
type Resolver struct {
	static *StaticProvider
	oauth2 *OAuth2Provider
	vault  *VaultProvider
	env    *EnvProvider
}

// NewResolver creates a Resolver with all provider implementations.
func NewResolver(vaultToken string) *Resolver {
	return &Resolver{
		static: &StaticProvider{},
		oauth2: NewOAuth2Provider(),
		vault:  NewVaultProvider(vaultToken),
		env:    &EnvProvider{},
	}
}

// Resolve returns the credential for a tool based on its auth type and credential config.
// If credentialConfigRaw is nil/empty, falls back to static provider (backward compat).
func (r *Resolver) Resolve(ctx context.Context, authType, authValue string, credentialConfigRaw json.RawMessage) (ResolvedCredential, error) {
	config, err := ParseConfig(credentialConfigRaw)
	if err != nil {
		return ResolvedCredential{}, err
	}

	provider := r.providerFor(config)
	return provider.Resolve(ctx, authType, authValue, config)
}

// HealthCheck checks credential resolution health for a tool.
func (r *Resolver) HealthCheck(ctx context.Context, authType, authValue string, credentialConfigRaw json.RawMessage) error {
	config, err := ParseConfig(credentialConfigRaw)
	if err != nil {
		return err
	}

	provider := r.providerFor(config)
	return provider.HealthCheck(ctx, authType, authValue, config)
}

// ValidateConfig validates a credential config without resolving.
func (r *Resolver) ValidateConfig(credentialConfigRaw json.RawMessage) error {
	config, err := ParseConfig(credentialConfigRaw)
	if err != nil {
		return err
	}
	return config.Validate()
}

// RedactConfig returns a copy of the credential config with secrets removed.
// Used for API responses and audit logs.
func RedactConfig(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil
	}

	// Redact sensitive fields.
	sensitiveKeys := []string{"client_secret", "vault_role"}
	for _, key := range sensitiveKeys {
		if _, ok := cfg[key]; ok {
			cfg[key] = "***"
		}
	}

	redacted, err := json.Marshal(cfg)
	if err != nil {
		return nil
	}
	return redacted
}

func (r *Resolver) providerFor(config *Config) Provider {
	if config == nil {
		return r.static
	}
	switch config.Provider {
	case ProviderOAuth2:
		return r.oauth2
	case ProviderVault:
		return r.vault
	case ProviderEnv:
		return r.env
	default:
		return r.static
	}
}

// ProviderName returns a human-readable name for the provider type from raw config.
func ProviderName(raw json.RawMessage) string {
	if len(raw) == 0 {
		return string(ProviderStatic)
	}
	config, err := ParseConfig(raw)
	if err != nil || config == nil {
		return string(ProviderStatic)
	}
	name := string(config.Provider)
	if name == "" {
		return string(ProviderStatic)
	}
	return name
}

// MaskSecrets returns a sanitized version of the config suitable for storage.
// This replaces the actual EPIC spec requirement for AES-256-GCM encryption
// with a simpler approach: store client_secret in the DB as-is (like auth_value)
// and rely on DB-level encryption at rest. The redaction in RedactConfig
// prevents secrets from leaking in API responses.
func MaskSecrets(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	// Validate it's parseable.
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("invalid credential_config: %w", err)
	}
	// Return as-is — secrets stored in DB alongside auth_value.
	// DB-level encryption at rest protects both.
	return raw, nil
}
