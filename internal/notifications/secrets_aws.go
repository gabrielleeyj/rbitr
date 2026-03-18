package notifications

import (
	"context"
	"fmt"
	"strings"
)

const awsSMPrefix = "aws-sm://"

// AWSSecretsManagerClient abstracts the AWS Secrets Manager API for testability.
type AWSSecretsManagerClient interface {
	GetSecretValue(ctx context.Context, secretID string) (string, error)
}

// AWSSecretsManagerProvider resolves secret refs with the "aws-sm://" prefix
// using AWS Secrets Manager.
type AWSSecretsManagerProvider struct {
	client AWSSecretsManagerClient
}

// NewAWSSecretsManagerProvider creates a provider that resolves aws-sm:// refs.
func NewAWSSecretsManagerProvider(client AWSSecretsManagerClient) *AWSSecretsManagerProvider {
	return &AWSSecretsManagerProvider{client: client}
}

func (p *AWSSecretsManagerProvider) Match(ref string) bool {
	return strings.HasPrefix(ref, awsSMPrefix)
}

func (p *AWSSecretsManagerProvider) Resolve(ctx context.Context, ref string) (string, error) {
	key := strings.TrimPrefix(ref, awsSMPrefix)
	if key == "" {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
	}

	value, err := p.client.GetSecretValue(ctx, key)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
	}
	return value, nil
}
