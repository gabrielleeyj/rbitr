package credential

import "context"

// StaticProvider resolves credentials from the tool's stored auth_value.
// This preserves backward compatibility with the existing behavior.
type StaticProvider struct{}

func (p *StaticProvider) Resolve(_ context.Context, authType, authValue string, _ *Config) (ResolvedCredential, error) {
	return resolveFromValue(authType, authValue), nil
}

func (p *StaticProvider) HealthCheck(_ context.Context, authType, authValue string, _ *Config) error {
	if authType != "none" && authValue == "" {
		return ErrNoCredential
	}
	return nil
}

// resolveFromValue maps an auth type and raw value to the appropriate HTTP header.
func resolveFromValue(authType, value string) ResolvedCredential {
	if value == "" {
		return ResolvedCredential{}
	}
	switch authType {
	case "bearer":
		return ResolvedCredential{
			HeaderName:  "Authorization",
			HeaderValue: "Bearer " + value,
		}
	case "api_key":
		return ResolvedCredential{
			HeaderName:  "X-Api-Key",
			HeaderValue: value,
		}
	default:
		return ResolvedCredential{}
	}
}
