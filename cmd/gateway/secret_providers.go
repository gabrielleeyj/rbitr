package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"

	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/notifications"
)

// awsSMClient wraps the AWS Secrets Manager SDK client.
type awsSMClient struct {
	client *secretsmanager.Client
}

func (a *awsSMClient) GetSecretValue(ctx context.Context, secretID string) (string, error) {
	output, err := a.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: &secretID,
	})
	if err != nil {
		return "", err
	}
	if output.SecretString == nil {
		return "", fmt.Errorf("secret %q has no string value (binary secrets not supported)", secretID)
	}
	return *output.SecretString, nil
}

// gcpSMClient uses the GCP Secret Manager REST API via raw HTTP
// to avoid heavy google-cloud-go SDK dependencies.
type gcpSMClient struct {
	client *http.Client
	token  string
}

func newGCPSMClient() (*gcpSMClient, error) {
	token := os.Getenv("GCP_SECRET_MANAGER_TOKEN")
	if token == "" {
		token = fetchGCPMetadataToken()
	}
	if token == "" {
		return nil, errors.New("GCP access token not available (set GCP_SECRET_MANAGER_TOKEN or run on GCE/GKE)")
	}
	return &gcpSMClient{client: http.DefaultClient, token: token}, nil
}

func fetchGCPMetadataToken() string {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token",
		http.NoBody)
	if err != nil {
		return ""
	}
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return ""
	}
	return tokenResp.AccessToken
}

func (g *gcpSMClient) AccessSecretVersion(ctx context.Context, name string) ([]byte, error) {
	url := fmt.Sprintf("https://secretmanager.googleapis.com/v1/%s:access", name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GCP Secret Manager API error: status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Payload struct {
			Data string `json:"data"`
		} `json:"payload"`
	}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&result); decodeErr != nil {
		return nil, decodeErr
	}

	decoded, err := base64.StdEncoding.DecodeString(result.Payload.Data)
	if err != nil {
		return []byte(result.Payload.Data), nil
	}
	return decoded, nil
}

// azureKVClient uses the Azure Key Vault REST API via raw HTTP
// to avoid heavy Azure SDK dependencies.
type azureKVClient struct {
	client *http.Client
	token  string
}

func newAzureKVClient() (*azureKVClient, error) {
	token := os.Getenv("AZURE_KEY_VAULT_TOKEN")
	if token == "" {
		return nil, errors.New("azure access token not available (set AZURE_KEY_VAULT_TOKEN)")
	}
	return &azureKVClient{client: http.DefaultClient, token: token}, nil
}

func (a *azureKVClient) GetSecret(ctx context.Context, vaultURL, secretName string) (string, error) {
	url := fmt.Sprintf("%s/secrets/%s?api-version=7.4", strings.TrimRight(vaultURL, "/"), secretName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+a.token)

	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("azure key vault API error: status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Value, nil
}

func appendCloudSecretProviders(providers []notifications.SecretProvider, cfg *config.Config) []notifications.SecretProvider {
	if cfg.SecretProviderAWS {
		awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
		if err != nil {
			log.Printf("WARNING: AWS secret provider enabled but SDK config failed: %v", err)
		} else {
			client := secretsmanager.NewFromConfig(awsCfg)
			providers = append(providers, notifications.NewAWSSecretsManagerProvider(&awsSMClient{client: client}))
			log.Println("secret provider: AWS Secrets Manager enabled")
		}
	}

	if cfg.SecretProviderGCP {
		gcpClient, err := newGCPSMClient()
		if err != nil {
			log.Printf("WARNING: GCP secret provider enabled but init failed: %v", err)
		} else {
			providers = append(providers, notifications.NewGCPSecretManagerProvider(gcpClient))
			log.Println("secret provider: GCP Secret Manager enabled")
		}
	}

	if cfg.SecretProviderVault {
		addr := cfg.SecretProviderVaultAddr
		token := os.Getenv("VAULT_TOKEN")
		if addr == "" || token == "" {
			log.Println("WARNING: Vault secret provider enabled but VAULT_ADDR or VAULT_TOKEN not set")
		} else {
			providers = append(providers, notifications.NewVaultProvider(addr, token))
			log.Println("secret provider: HashiCorp Vault enabled")
		}
	}

	if cfg.SecretProviderAzure {
		azureClient, err := newAzureKVClient()
		if err != nil {
			log.Printf("WARNING: Azure secret provider enabled but init failed: %v", err)
		} else {
			providers = append(providers, notifications.NewAzureKeyVaultProvider(azureClient))
			log.Println("secret provider: Azure Key Vault enabled")
		}
	}

	return providers
}
