package credential

import (
	"context"
	"encoding/json"
	"fmt"
)

// ProviderType identifies which credential provider to use.
type ProviderType string

const (
	ProviderStatic ProviderType = "static"
	ProviderOAuth2 ProviderType = "oauth2_client_credentials"
	ProviderVault  ProviderType = "vault"
	ProviderEnv    ProviderType = "env"
)

// ResolvedCredential contains the resolved auth header key and value.
type ResolvedCredential struct {
	HeaderName  string // e.g. "Authorization" or "X-Api-Key"
	HeaderValue string // e.g. "Bearer <token>"
}

// Provider resolves credentials for a tool at request time.
type Provider interface {
	// Resolve returns the current credential for the given auth type.
	// Returns empty ResolvedCredential if no credential is configured.
	Resolve(ctx context.Context, authType, authValue string, config *Config) (ResolvedCredential, error)

	// HealthCheck validates that the credential provider can resolve credentials.
	HealthCheck(ctx context.Context, authType, authValue string, config *Config) error
}

// Config holds the credential provider configuration stored in credential_config JSONB.
type Config struct {
	Provider ProviderType `json:"provider"`

	// OAuth2 client credentials fields.
	TokenURL      string `json:"token_url,omitempty"`
	ClientID      string `json:"client_id,omitempty"`
	ClientSecret  string `json:"client_secret,omitempty"`
	Scope         string `json:"scope,omitempty"`
	TokenCacheTTL int    `json:"token_cache_ttl_seconds,omitempty"`

	// Vault fields.
	VaultAddr string `json:"vault_addr,omitempty"`
	VaultPath string `json:"vault_path,omitempty"`
	VaultRole string `json:"vault_role,omitempty"`

	// Env fields.
	EnvVar string `json:"env_var,omitempty"`
}

// ParseConfig parses a credential config from raw JSON.
// Returns nil if the input is nil or empty.
func ParseConfig(raw json.RawMessage) (*Config, error) {
	if len(raw) == 0 {
		return nil, nil //nolint:nilnil // nil config means "no config, use static"
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("invalid credential_config: %w", err)
	}
	return &cfg, nil
}

// Validate checks that the config has the required fields for its provider type.
func (c *Config) Validate() error {
	if c == nil {
		return nil
	}
	switch c.Provider {
	case ProviderStatic, "":
		return nil
	case ProviderOAuth2:
		if c.TokenURL == "" {
			return fmt.Errorf("token_url is required for %s provider", c.Provider)
		}
		if c.ClientID == "" {
			return fmt.Errorf("client_id is required for %s provider", c.Provider)
		}
		if c.ClientSecret == "" {
			return fmt.Errorf("client_secret is required for %s provider", c.Provider)
		}
		return nil
	case ProviderVault:
		if c.VaultAddr == "" {
			return fmt.Errorf("vault_addr is required for %s provider", c.Provider)
		}
		if c.VaultPath == "" {
			return fmt.Errorf("vault_path is required for %s provider", c.Provider)
		}
		return nil
	case ProviderEnv:
		if c.EnvVar == "" {
			return fmt.Errorf("env_var is required for %s provider", c.Provider)
		}
		return nil
	default:
		return fmt.Errorf("unknown credential provider: %s", c.Provider)
	}
}
