package credential

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const defaultVaultCacheTTL = 60 * time.Second

// vaultSecret represents a cached Vault secret value.
type vaultSecret struct {
	value     string
	expiresAt time.Time
}

// VaultProvider resolves credentials from HashiCorp Vault.
type VaultProvider struct {
	mu     sync.RWMutex
	cache  map[string]*vaultSecret // keyed by "vaultAddr|vaultPath"
	client *http.Client
	token  string // Vault token (from VAULT_TOKEN env var or AppRole)
}

// NewVaultProvider creates a new Vault credential provider.
func NewVaultProvider(vaultToken string) *VaultProvider {
	return &VaultProvider{
		cache:  make(map[string]*vaultSecret),
		client: &http.Client{Timeout: httpTimeout},
		token:  vaultToken,
	}
}

func (p *VaultProvider) Resolve(ctx context.Context, authType, _ string, config *Config) (ResolvedCredential, error) {
	if config == nil {
		return ResolvedCredential{}, ErrNoCredential
	}

	secret, err := p.getSecret(ctx, config)
	if err != nil {
		return ResolvedCredential{}, err
	}

	return resolveFromValue(authType, secret), nil
}

func (p *VaultProvider) HealthCheck(ctx context.Context, authType, _ string, config *Config) error {
	if config == nil {
		return ErrNoCredential
	}
	_, err := p.getSecret(ctx, config)
	return err
}

func (p *VaultProvider) getSecret(ctx context.Context, config *Config) (string, error) {
	cacheKey := config.VaultAddr + "|" + config.VaultPath

	p.mu.RLock()
	if s, ok := p.cache[cacheKey]; ok && time.Now().Before(s.expiresAt) {
		p.mu.RUnlock()
		return s.value, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock.
	if s, ok := p.cache[cacheKey]; ok && time.Now().Before(s.expiresAt) {
		return s.value, nil
	}

	secret, err := p.fetchSecret(ctx, config)
	if err != nil {
		return "", err
	}

	p.cache[cacheKey] = secret
	return secret.value, nil
}

func (p *VaultProvider) fetchSecret(ctx context.Context, config *Config) (*vaultSecret, error) {
	reqURL := config.VaultAddr + "/v1/" + config.VaultPath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrVaultUnavailable, err.Error())
	}
	req.Header.Set("X-Vault-Token", p.token)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrVaultUnavailable, err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return nil, fmt.Errorf("%w: vault returned %d: %s", ErrVaultUnavailable, resp.StatusCode, string(body))
	}

	var vaultResp struct {
		Data struct {
			Data map[string]string `json:"data"` // KV v2
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&vaultResp); err != nil {
		return nil, fmt.Errorf("%w: invalid vault response: %s", ErrVaultUnavailable, err.Error())
	}

	// Look for "value" key in the secret data.
	value, ok := vaultResp.Data.Data["value"]
	if !ok {
		// Fallback: use first key's value.
		for _, v := range vaultResp.Data.Data {
			value = v
			break
		}
	}

	if value == "" {
		return nil, fmt.Errorf("%w: empty secret at path %s", ErrVaultUnavailable, config.VaultPath)
	}

	return &vaultSecret{
		value:     value,
		expiresAt: time.Now().Add(defaultVaultCacheTTL),
	}, nil
}
