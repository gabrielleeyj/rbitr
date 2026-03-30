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

// ProviderConfig holds configuration for creating a LicenseProvider and UsageReporter.
type ProviderConfig struct {
	// Name selects the provider implementation (e.g. "self-managed", "aws-marketplace").
	Name string

	// PubKey is the Ed25519 public key for JWT license validation (self-managed only).
	PubKey ed25519.PublicKey

	// KeyPath is the path to the license key file on disk (self-managed only).
	KeyPath string

	// AWSProductCode is the AWS Marketplace product code.
	AWSProductCode string

	// AWSRegion overrides the default AWS region for marketplace API calls.
	AWSRegion string

	// AWSCustomerID is a pre-resolved customer identifier (optional; can be resolved at runtime via activation endpoint).
	AWSCustomerID string
}

// NewProvider creates a LicenseProvider and UsageReporter based on the given config.
// For self-managed, constructs the provider inline. For marketplace providers,
// returns an error indicating they must be constructed via their dedicated packages.
func NewProvider(cfg *ProviderConfig) (LicenseProvider, UsageReporter, error) {
	switch cfg.Name {
	case ProviderSelfManaged, "":
		validator := NewValidator(cfg.PubKey, cfg.KeyPath)
		validator.LoadAndValidate()
		watcher := NewWatcher(validator, cfg.KeyPath)
		provider := NewSelfManagedProvider(validator, watcher)
		return provider, &NoopReporter{}, nil

	case ProviderAWSMarketplace, ProviderGCPMarketplace, ProviderAzureMarketplace:
		return nil, nil, fmt.Errorf("license provider %q must be constructed via its dedicated package", cfg.Name)

	default:
		return nil, nil, fmt.Errorf("unknown license provider: %q (supported: %s, %s, %s, %s)",
			cfg.Name, ProviderSelfManaged, ProviderAWSMarketplace, ProviderGCPMarketplace, ProviderAzureMarketplace)
	}
}
