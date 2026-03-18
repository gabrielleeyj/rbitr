package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const vaultPrefix = "vault://"

// VaultProvider resolves secret refs with the "vault://" prefix using
// HashiCorp Vault's KV v2 HTTP API.
type VaultProvider struct {
	addr   string
	token  string
	client *http.Client
}

// NewVaultProvider creates a provider that resolves vault:// refs.
// addr is the Vault server address (e.g., "https://vault.example.com:8200").
// token is the Vault authentication token.
func NewVaultProvider(addr, token string) *VaultProvider {
	return &VaultProvider{
		addr:   strings.TrimRight(addr, "/"),
		token:  token,
		client: http.DefaultClient,
	}
}

func (p *VaultProvider) Match(ref string) bool {
	return strings.HasPrefix(ref, vaultPrefix)
}

func (p *VaultProvider) Resolve(ctx context.Context, ref string) (string, error) {
	raw := strings.TrimPrefix(ref, vaultPrefix)
	if raw == "" {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
	}

	// Split path#key — optional key selector for multi-field KV entries.
	path, keyName, _ := strings.Cut(raw, "#")
	if path == "" {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
	}

	url := fmt.Sprintf("%s/v1/%s", p.addr, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
	}
	req.Header.Set("X-Vault-Token", p.token)

	client := p.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
	}

	return extractVaultKV2Value(body, keyName, ref)
}

// extractVaultKV2Value parses a Vault KV v2 JSON response and extracts
// the requested key from data.data. If keyName is empty and the map has
// exactly one key, that value is returned.
func extractVaultKV2Value(body []byte, keyName, ref string) (string, error) {
	var envelope struct {
		Data struct {
			Data map[string]any `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
	}
	dataMap := envelope.Data.Data
	if len(dataMap) == 0 {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
	}

	if keyName != "" {
		val, ok := dataMap[keyName]
		if !ok {
			return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
		}
		s, ok := val.(string)
		if !ok {
			return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
		}
		return s, nil
	}

	// No key specified — if exactly one key, return it.
	if len(dataMap) == 1 {
		for _, val := range dataMap {
			s, ok := val.(string)
			if !ok {
				return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
			}
			return s, nil
		}
	}

	return "", errors.New("vault secret has multiple keys; specify one with vault://path#key")
}
