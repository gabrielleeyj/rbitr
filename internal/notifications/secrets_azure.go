package notifications

import (
	"context"
	"fmt"
	"strings"
)

const azureKVPrefix = "azure-kv://"

// AzureKeyVaultClient abstracts the Azure Key Vault API for testability.
type AzureKeyVaultClient interface {
	GetSecret(ctx context.Context, vaultURL, secretName string) (string, error)
}

// AzureKeyVaultProvider resolves secret refs with the "azure-kv://" prefix
// using Azure Key Vault.
// Ref format: azure-kv://<vault-name>/<secret-name>
type AzureKeyVaultProvider struct {
	client AzureKeyVaultClient
}

// NewAzureKeyVaultProvider creates a provider that resolves azure-kv:// refs.
func NewAzureKeyVaultProvider(client AzureKeyVaultClient) *AzureKeyVaultProvider {
	return &AzureKeyVaultProvider{client: client}
}

func (p *AzureKeyVaultProvider) Match(ref string) bool {
	return strings.HasPrefix(ref, azureKVPrefix)
}

func (p *AzureKeyVaultProvider) Resolve(ctx context.Context, ref string) (string, error) {
	raw := strings.TrimPrefix(ref, azureKVPrefix)
	if raw == "" {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
	}

	// Split into vault name and secret name.
	vaultName, secretName, ok := strings.Cut(raw, "/")
	if !ok || vaultName == "" || secretName == "" {
		return "", fmt.Errorf("%w: %s (expected azure-kv://<vault>/<secret>)", ErrSecretNotFound, redactRef(ref))
	}

	vaultURL := fmt.Sprintf("https://%s.vault.azure.net", vaultName)
	value, err := p.client.GetSecret(ctx, vaultURL, secretName)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
	}
	return value, nil
}
