package credential

import "errors"

var (
	// ErrNoCredential indicates no credential is configured for a tool.
	ErrNoCredential = errors.New("no credential configured")

	// ErrTokenRefreshFailed indicates an OAuth2 token refresh failed.
	ErrTokenRefreshFailed = errors.New("token refresh failed")

	// ErrVaultUnavailable indicates the Vault server is unreachable.
	ErrVaultUnavailable = errors.New("vault unavailable")

	// ErrEnvVarNotSet indicates the configured environment variable is not set.
	ErrEnvVarNotSet = errors.New("environment variable not set")
)
