package credential

import (
	"context"
	"fmt"
	"os"
)

// EnvProvider resolves credentials from environment variables.
type EnvProvider struct{}

func (p *EnvProvider) Resolve(_ context.Context, authType, _ string, config *Config) (ResolvedCredential, error) {
	if config == nil {
		return ResolvedCredential{}, ErrNoCredential
	}

	value := os.Getenv(config.EnvVar)
	if value == "" {
		return ResolvedCredential{}, fmt.Errorf("%w: %s", ErrEnvVarNotSet, config.EnvVar)
	}

	return resolveFromValue(authType, value), nil
}

func (p *EnvProvider) HealthCheck(_ context.Context, _, _ string, config *Config) error {
	if config == nil {
		return ErrNoCredential
	}
	if os.Getenv(config.EnvVar) == "" {
		return fmt.Errorf("%w: %s", ErrEnvVarNotSet, config.EnvVar)
	}
	return nil
}
