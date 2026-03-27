package license

import (
	"crypto/ed25519"
	"fmt"
)

const (
	// ProviderSelfManaged is the default license provider using JWT files on disk.
	ProviderSelfManaged = "self-managed"

	// ProviderAWSMarketplace uses AWS Marketplace metering and entitlements.
	ProviderAWSMarketplace = "aws-marketplace"

	// ProviderGCPMarketplace uses GCP Cloud Commerce Procurement API.
	ProviderGCPMarketplace = "gcp-marketplace"

	// ProviderAzureMarketplace uses Azure SaaS Fulfillment and Metering APIs.
	ProviderAzureMarketplace = "azure-marketplace"
)

// NewProvider creates a LicenseProvider and UsageReporter for the given
// provider name. Only "self-managed" is currently supported; marketplace
// providers will be added in future phases.
func NewProvider(providerName string, pubKey ed25519.PublicKey, keyPath string) (LicenseProvider, UsageReporter, error) {
	switch providerName {
	case ProviderSelfManaged, "":
		validator := NewValidator(pubKey, keyPath)
		validator.LoadAndValidate()
		watcher := NewWatcher(validator, keyPath)
		provider := NewSelfManagedProvider(validator, watcher)
		return provider, &NoopReporter{}, nil

	case ProviderAWSMarketplace, ProviderGCPMarketplace, ProviderAzureMarketplace:
		return nil, nil, fmt.Errorf("license provider %q is not yet implemented", providerName)

	default:
		return nil, nil, fmt.Errorf("unknown license provider: %q (supported: %s, %s, %s, %s)",
			providerName, ProviderSelfManaged, ProviderAWSMarketplace, ProviderGCPMarketplace, ProviderAzureMarketplace)
	}
}
