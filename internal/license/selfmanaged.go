package license

import "context"

// SelfManagedProvider wraps the existing Validator and Watcher to implement
// LicenseProvider and SelfManagedManager. This is the default provider for
// installations that use a JWT license key file on disk.
type SelfManagedProvider struct {
	validator *Validator
	watcher   *Watcher
}

// Compile-time interface checks.
var (
	_ LicenseProvider    = (*SelfManagedProvider)(nil)
	_ SelfManagedManager = (*SelfManagedProvider)(nil)
)

// NewSelfManagedProvider creates a provider that delegates to the given
// Validator and Watcher.
func NewSelfManagedProvider(validator *Validator, watcher *Watcher) *SelfManagedProvider {
	return &SelfManagedProvider{
		validator: validator,
		watcher:   watcher,
	}
}

// Info returns the current license info.
func (p *SelfManagedProvider) Info() LicenseInfo {
	return p.validator.Info()
}

// Entitlements returns the current resolved entitlements.
func (p *SelfManagedProvider) Entitlements() Entitlements {
	return p.validator.Entitlements()
}

// Start validates the license on boot and then starts the file watcher.
// It blocks until ctx is cancelled.
func (p *SelfManagedProvider) Start(ctx context.Context) {
	p.watcher.Start(ctx)
}

// ValidateBytes parses and validates a raw license key token.
func (p *SelfManagedProvider) ValidateBytes(data []byte) (LicenseInfo, error) {
	return p.validator.ValidateBytes(data)
}

// KeyPath returns the configured license key file path.
func (p *SelfManagedProvider) KeyPath() string {
	return p.validator.KeyPath()
}

// LoadAndValidate re-reads and validates the license key file.
func (p *SelfManagedProvider) LoadAndValidate() {
	p.validator.LoadAndValidate()
}
