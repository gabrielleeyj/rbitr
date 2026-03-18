package notifications

import (
	"context"
	"errors"
	"testing"
)

type mockAzureClient struct {
	secrets map[string]map[string]string // vaultURL -> secretName -> value
	err     error
}

func (m *mockAzureClient) GetSecret(_ context.Context, vaultURL, secretName string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	vault, ok := m.secrets[vaultURL]
	if !ok {
		return "", errors.New("vault not found")
	}
	val, ok := vault[secretName]
	if !ok {
		return "", errors.New("SecretNotFound")
	}
	return val, nil
}

func TestAzureProviderMatch(t *testing.T) {
	p := NewAzureKeyVaultProvider(nil)
	if !p.Match("azure-kv://myvault/slack-token") {
		t.Fatal("expected match for azure-kv:// prefix")
	}
	if p.Match("vault://secret/data/key") {
		t.Fatal("unexpected match for vault:// prefix")
	}
	if p.Match("env://KEY") {
		t.Fatal("unexpected match for env:// prefix")
	}
}

func TestAzureProviderResolve(t *testing.T) {
	client := &mockAzureClient{secrets: map[string]map[string]string{
		"https://myvault.vault.azure.net": {
			"slack-token": "azure-secret-value",
		},
	}}
	p := NewAzureKeyVaultProvider(client)

	val, err := p.Resolve(context.Background(), "azure-kv://myvault/slack-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "azure-secret-value" {
		t.Fatalf("expected azure-secret-value, got %q", val)
	}
}

func TestAzureProviderResolveNotFound(t *testing.T) {
	client := &mockAzureClient{secrets: map[string]map[string]string{
		"https://myvault.vault.azure.net": {},
	}}
	p := NewAzureKeyVaultProvider(client)

	_, err := p.Resolve(context.Background(), "azure-kv://myvault/missing-secret")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestAzureProviderResolveEmptyRef(t *testing.T) {
	p := NewAzureKeyVaultProvider(nil)

	_, err := p.Resolve(context.Background(), "azure-kv://")
	if err == nil {
		t.Fatal("expected error for empty ref")
	}
	if !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestAzureProviderResolveMissingSecretName(t *testing.T) {
	p := NewAzureKeyVaultProvider(nil)

	_, err := p.Resolve(context.Background(), "azure-kv://myvault")
	if err == nil {
		t.Fatal("expected error for missing secret name")
	}
	if !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestAzureProviderResolveClientError(t *testing.T) {
	client := &mockAzureClient{err: errors.New("access denied")}
	p := NewAzureKeyVaultProvider(client)

	_, err := p.Resolve(context.Background(), "azure-kv://myvault/token")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestAzureProviderResolveRedactsRef(t *testing.T) {
	client := &mockAzureClient{secrets: map[string]map[string]string{}}
	p := NewAzureKeyVaultProvider(client)

	_, err := p.Resolve(context.Background(), "azure-kv://myvault/very-long-secret-name-that-should-be-redacted")
	if err == nil {
		t.Fatal("expected error")
	}
	errMsg := err.Error()
	if len(errMsg) > 80 {
		t.Fatalf("error message too long, may leak secret path: %q", errMsg)
	}
}
