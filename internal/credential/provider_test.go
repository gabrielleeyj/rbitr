package credential

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStaticProvider_Resolve(t *testing.T) {
	t.Parallel()
	p := &StaticProvider{}

	cases := []struct {
		name       string
		authType   string
		authValue  string
		wantHeader string
		wantValue  string
	}{
		{"bearer token", "bearer", "tok123", "Authorization", "Bearer tok123"},
		{"api key", "api_key", "key456", "X-Api-Key", "key456"},
		{"none type", "none", "", "", ""},
		{"empty value", "bearer", "", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cred, err := p.Resolve(context.Background(), tc.authType, tc.authValue, nil)
			require.NoError(t, err)
			require.Equal(t, tc.wantHeader, cred.HeaderName)
			require.Equal(t, tc.wantValue, cred.HeaderValue)
		})
	}
}

func TestStaticProvider_HealthCheck(t *testing.T) {
	t.Parallel()
	p := &StaticProvider{}

	require.NoError(t, p.HealthCheck(context.Background(), "none", "", nil))
	require.NoError(t, p.HealthCheck(context.Background(), "bearer", "tok", nil))
	require.ErrorIs(t, p.HealthCheck(context.Background(), "bearer", "", nil), ErrNoCredential)
}

func TestEnvProvider_Resolve(t *testing.T) {
	p := &EnvProvider{}

	t.Run("env var set", func(t *testing.T) {
		t.Setenv("TEST_CRED_VAR", "my-secret-key")
		cfg := &Config{Provider: ProviderEnv, EnvVar: "TEST_CRED_VAR"}
		cred, err := p.Resolve(context.Background(), "api_key", "", cfg)
		require.NoError(t, err)
		require.Equal(t, "X-Api-Key", cred.HeaderName)
		require.Equal(t, "my-secret-key", cred.HeaderValue)
	})

	t.Run("env var not set", func(t *testing.T) {
		cfg := &Config{Provider: ProviderEnv, EnvVar: "NONEXISTENT_VAR_12345"}
		_, err := p.Resolve(context.Background(), "bearer", "", cfg)
		require.ErrorIs(t, err, ErrEnvVarNotSet)
	})

	t.Run("nil config", func(t *testing.T) {
		_, err := p.Resolve(context.Background(), "bearer", "", nil)
		require.ErrorIs(t, err, ErrNoCredential)
	})
}

func TestEnvProvider_HealthCheck(t *testing.T) {
	p := &EnvProvider{}

	t.Run("healthy", func(t *testing.T) {
		t.Setenv("TEST_HC_VAR", "value")
		cfg := &Config{Provider: ProviderEnv, EnvVar: "TEST_HC_VAR"}
		require.NoError(t, p.HealthCheck(context.Background(), "", "", cfg))
	})

	t.Run("unhealthy", func(t *testing.T) {
		cfg := &Config{Provider: ProviderEnv, EnvVar: "MISSING_HC_VAR_99"}
		require.ErrorIs(t, p.HealthCheck(context.Background(), "", "", cfg), ErrEnvVarNotSet)
	})
}

func TestOAuth2Provider_Resolve(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
			require.NoError(t, r.ParseForm())
			require.Equal(t, "client_credentials", r.Form.Get("grant_type"))
			require.Equal(t, "my-client", r.Form.Get("client_id"))
			require.Equal(t, "my-secret", r.Form.Get("client_secret"))
			require.Equal(t, "api:read", r.Form.Get("scope"))

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "fresh-token-123",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		}))
		defer srv.Close()

		p := NewOAuth2Provider()
		cfg := &Config{
			Provider:      ProviderOAuth2,
			TokenURL:      srv.URL,
			ClientID:      "my-client",
			ClientSecret:  "my-secret",
			Scope:         "api:read",
			TokenCacheTTL: 60,
		}

		cred, err := p.Resolve(context.Background(), "", "", cfg)
		require.NoError(t, err)
		require.Equal(t, "Authorization", cred.HeaderName)
		require.Equal(t, "Bearer fresh-token-123", cred.HeaderValue)

		// Second call should use cache.
		cred2, err := p.Resolve(context.Background(), "", "", cfg)
		require.NoError(t, err)
		require.Equal(t, "Bearer fresh-token-123", cred2.HeaderValue)
	})

	t.Run("token endpoint error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error": "invalid_client"}`))
		}))
		defer srv.Close()

		p := NewOAuth2Provider()
		cfg := &Config{
			Provider:     ProviderOAuth2,
			TokenURL:     srv.URL,
			ClientID:     "bad-client",
			ClientSecret: "bad-secret",
		}

		_, err := p.Resolve(context.Background(), "", "", cfg)
		require.ErrorIs(t, err, ErrTokenRefreshFailed)
	})

	t.Run("nil config", func(t *testing.T) {
		p := NewOAuth2Provider()
		_, err := p.Resolve(context.Background(), "", "", nil)
		require.ErrorIs(t, err, ErrNoCredential)
	})
}

func TestVaultProvider_Resolve(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "test-vault-token", r.Header.Get("X-Vault-Token"))
			require.Equal(t, "/v1/secret/data/myapp/cred", r.URL.Path)

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"data": map[string]string{
						"value": "vault-secret-123",
					},
				},
			})
		}))
		defer srv.Close()

		p := NewVaultProvider("test-vault-token")
		cfg := &Config{
			Provider:  ProviderVault,
			VaultAddr: srv.URL,
			VaultPath: "secret/data/myapp/cred",
		}

		cred, err := p.Resolve(context.Background(), "bearer", "", cfg)
		require.NoError(t, err)
		require.Equal(t, "Authorization", cred.HeaderName)
		require.Equal(t, "Bearer vault-secret-123", cred.HeaderValue)
	})

	t.Run("vault error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"errors": ["permission denied"]}`))
		}))
		defer srv.Close()

		p := NewVaultProvider("bad-token")
		cfg := &Config{
			Provider:  ProviderVault,
			VaultAddr: srv.URL,
			VaultPath: "secret/data/myapp/cred",
		}

		_, err := p.Resolve(context.Background(), "bearer", "", cfg)
		require.ErrorIs(t, err, ErrVaultUnavailable)
	})
}

func TestResolver_Resolve(t *testing.T) {
	t.Run("nil config uses static", func(t *testing.T) {
		r := NewResolver("")
		cred, err := r.Resolve(context.Background(), "bearer", "static-tok", nil)
		require.NoError(t, err)
		require.Equal(t, "Authorization", cred.HeaderName)
		require.Equal(t, "Bearer static-tok", cred.HeaderValue)
	})

	t.Run("empty config uses static", func(t *testing.T) {
		r := NewResolver("")
		cred, err := r.Resolve(context.Background(), "api_key", "key123", json.RawMessage(`{}`))
		require.NoError(t, err)
		require.Equal(t, "X-Api-Key", cred.HeaderName)
		require.Equal(t, "key123", cred.HeaderValue)
	})

	t.Run("env provider", func(t *testing.T) {
		t.Setenv("TEST_RESOLVER_VAR", "env-token")
		r := NewResolver("")
		cfg := json.RawMessage(`{"provider": "env", "env_var": "TEST_RESOLVER_VAR"}`)
		cred, err := r.Resolve(context.Background(), "bearer", "", cfg)
		require.NoError(t, err)
		require.Equal(t, "Authorization", cred.HeaderName)
		require.Equal(t, "Bearer env-token", cred.HeaderValue)
	})

	t.Run("invalid config json", func(t *testing.T) {
		r := NewResolver("")
		_, err := r.Resolve(context.Background(), "bearer", "", json.RawMessage(`{invalid`))
		require.Error(t, err)
	})
}

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{"nil/empty is valid", Config{}, false},
		{"static is valid", Config{Provider: ProviderStatic}, false},
		{"oauth2 valid", Config{Provider: ProviderOAuth2, TokenURL: "https://auth.example.com/token", ClientID: "id", ClientSecret: "secret"}, false},
		{"oauth2 missing token_url", Config{Provider: ProviderOAuth2, ClientID: "id", ClientSecret: "secret"}, true},
		{"oauth2 missing client_id", Config{Provider: ProviderOAuth2, TokenURL: "https://auth.example.com/token", ClientSecret: "secret"}, true},
		{"oauth2 missing client_secret", Config{Provider: ProviderOAuth2, TokenURL: "https://auth.example.com/token", ClientID: "id"}, true},
		{"vault valid", Config{Provider: ProviderVault, VaultAddr: "http://vault:8200", VaultPath: "secret/data/cred"}, false},
		{"vault missing addr", Config{Provider: ProviderVault, VaultPath: "secret/data/cred"}, true},
		{"vault missing path", Config{Provider: ProviderVault, VaultAddr: "http://vault:8200"}, true},
		{"env valid", Config{Provider: ProviderEnv, EnvVar: "MY_VAR"}, false},
		{"env missing var", Config{Provider: ProviderEnv}, true},
		{"unknown provider", Config{Provider: "magic"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRedactConfig(t *testing.T) {
	t.Parallel()

	t.Run("redacts secrets", func(t *testing.T) {
		raw := json.RawMessage(`{"provider":"oauth2_client_credentials","token_url":"https://auth.example.com","client_id":"id","client_secret":"super-secret","scope":"api:read"}`)
		redacted := RedactConfig(raw)
		require.NotNil(t, redacted)

		var m map[string]any
		require.NoError(t, json.Unmarshal(redacted, &m))
		require.Equal(t, "***", m["client_secret"])
		require.Equal(t, "id", m["client_id"])
		require.Equal(t, "https://auth.example.com", m["token_url"])
	})

	t.Run("nil returns nil", func(t *testing.T) {
		require.Nil(t, RedactConfig(nil))
	})

	t.Run("empty returns nil", func(t *testing.T) {
		require.Nil(t, RedactConfig(json.RawMessage(``)))
	})
}

func TestProviderName(t *testing.T) {
	t.Parallel()

	require.Equal(t, "static", ProviderName(nil))
	require.Equal(t, "static", ProviderName(json.RawMessage(`{}`)))
	require.Equal(t, "oauth2_client_credentials", ProviderName(json.RawMessage(`{"provider":"oauth2_client_credentials"}`)))
	require.Equal(t, "env", ProviderName(json.RawMessage(`{"provider":"env"}`)))
	require.Equal(t, "vault", ProviderName(json.RawMessage(`{"provider":"vault"}`)))
}
