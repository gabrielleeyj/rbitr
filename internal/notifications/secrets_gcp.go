package notifications

import (
	"context"
	"fmt"
	"strings"
)

const gcpSMPrefix = "gcp-sm://"

// GCPSecretManagerClient abstracts the GCP Secret Manager API for testability.
type GCPSecretManagerClient interface {
	AccessSecretVersion(ctx context.Context, name string) ([]byte, error)
}

// GCPSecretManagerProvider resolves secret refs with the "gcp-sm://" prefix
// using Google Cloud Secret Manager.
type GCPSecretManagerProvider struct {
	client GCPSecretManagerClient
}

// NewGCPSecretManagerProvider creates a provider that resolves gcp-sm:// refs.
func NewGCPSecretManagerProvider(client GCPSecretManagerClient) *GCPSecretManagerProvider {
	return &GCPSecretManagerProvider{client: client}
}

func (p *GCPSecretManagerProvider) Match(ref string) bool {
	return strings.HasPrefix(ref, gcpSMPrefix)
}

func (p *GCPSecretManagerProvider) Resolve(ctx context.Context, ref string) (string, error) {
	name := strings.TrimPrefix(ref, gcpSMPrefix)
	if name == "" {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
	}

	// Auto-append /versions/latest if no version segment is present.
	if !strings.Contains(name, "/versions/") {
		name += "/versions/latest"
	}

	data, err := p.client.AccessSecretVersion(ctx, name)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, redactRef(ref))
	}
	return string(data), nil
}
