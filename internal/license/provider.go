package license

import "context"

// LicenseProvider abstracts license entitlement resolution.
// Implementations: self-managed (JWT file), AWS Marketplace, GCP Marketplace, Azure Marketplace.
type LicenseProvider interface {
	// Info returns the current license metadata and entitlements.
	Info() LicenseInfo

	// Entitlements returns the current resolved entitlements (thread-safe).
	Entitlements() Entitlements

	// Start begins any background work (polling, cache refresh). Blocks until ctx is cancelled.
	Start(ctx context.Context)
}

// SelfManagedManager is an optional interface for providers that support
// manual license key upload and removal. Only the self-managed provider
// implements this — marketplace providers manage licensing externally.
type SelfManagedManager interface {
	ValidateBytes(data []byte) (LicenseInfo, error)
	KeyPath() string
	LoadAndValidate()
}
