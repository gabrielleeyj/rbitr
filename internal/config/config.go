package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL             string
	DBMaxOpenConns          int
	DBMaxIdleConns          int
	DBConnMaxLifetime       time.Duration
	DBConnMaxIdleTime       time.Duration
	ListenAddr              string
	BodyLimitSize           int64
	ResponseLimit           int64
	DisableXTenantKey       bool
	FeatureRateLimiting     bool
	FeatureArgConstraints   bool
	FeatureShadowMode       bool
	FeatureSessionTokens    bool
	FeatureFileGovernance   bool
	FeatureCrossTenantChain bool
	SessionTokenTTL         time.Duration
	MaxChainDepth           int
	SecretProviderAWS       bool
	SecretProviderGCP       bool
	SecretProviderVault     bool
	SecretProviderVaultAddr string
	SecretProviderAzure     bool
	SSOEnabled              bool
	SSOIssuer               string
	SSOClientID             string
	SSOClientSecretRef      string
	SSORedirectURI          string
	SSOAllowedDomains       []string
	SSODefaultScopes        []string
	LicenseKeyPath          string
	LicenseProvider         string
	AWSProductCode          string
	AWSMarketplaceRegion    string
	AWSCustomerID           string
	GCPProjectID            string
	GCPServiceName          string
	DevAutoTools            bool
	OutboundAllowPrivate    bool
	DevMockInternalURL      string
	DevJiraURL              string
	SetupTokenRequired      bool
	SetupToken              string
	SetupTokenFile          string
	SetupAllowedCIDRs       []string
}

const (
	defaultLimitBytes        = 256 * 1024
	defaultDBMaxOpenConns    = 30
	defaultDBMaxIdleConns    = 10
	defaultDBConnMaxLifetime = 30 * time.Minute
	defaultDBConnMaxIdleTime = 5 * time.Minute
	defaultSessionTokenTTL   = 15 * time.Minute
	defaultMaxChainDepth     = 5
)

func Load() Config {
	return Config{
		DatabaseURL:             getEnv("DATABASE_URL", "postgres://postgres@localhost:2345/rbitr?sslmode=require"),
		DBMaxOpenConns:          getEnvInt("DB_MAX_OPEN_CONNS", defaultDBMaxOpenConns),
		DBMaxIdleConns:          getEnvInt("DB_MAX_IDLE_CONNS", defaultDBMaxIdleConns),
		DBConnMaxLifetime:       getEnvDurationFromSeconds("DB_CONN_MAX_LIFETIME_SECONDS", defaultDBConnMaxLifetime),
		DBConnMaxIdleTime:       getEnvDurationFromSeconds("DB_CONN_MAX_IDLE_TIME_SECONDS", defaultDBConnMaxIdleTime),
		ListenAddr:              getEnv("LISTEN_ADDR", ":8080"),
		BodyLimitSize:           getEnvInt64("BODY_LIMIT_BYTES", defaultLimitBytes),
		ResponseLimit:           getEnvInt64("RESPONSE_LIMIT_BYTES", defaultLimitBytes),
		DisableXTenantKey:       getEnvBool("RBTR_DISABLE_X_TENANT_KEY"),
		FeatureRateLimiting:     getEnvBool("RBTR_FEATURE_RATE_LIMITING"),
		FeatureArgConstraints:   getEnvBool("RBTR_FEATURE_ARG_CONSTRAINTS"),
		FeatureShadowMode:       getEnvBool("RBTR_FEATURE_SHADOW_MODE"),
		FeatureSessionTokens:    getEnvBoolDefault("RBTR_FEATURE_SESSION_TOKENS", true),
		FeatureFileGovernance:   getEnvBoolDefault("RBTR_FEATURE_FILE_GOVERNANCE", true),
		FeatureCrossTenantChain: getEnvBool("RBTR_FEATURE_CROSS_TENANT_CHAIN"),
		MaxChainDepth:           getEnvInt("RBTR_MAX_CHAIN_DEPTH", defaultMaxChainDepth),
		SessionTokenTTL:         getEnvDurationFromSeconds("RBTR_SESSION_TOKEN_TTL_SECONDS", defaultSessionTokenTTL),
		SecretProviderAWS:       getEnvBool("RBTR_SECRET_PROVIDER_AWS"),
		SecretProviderGCP:       getEnvBool("RBTR_SECRET_PROVIDER_GCP"),
		SecretProviderVault:     getEnvBool("RBTR_SECRET_PROVIDER_VAULT"),
		SecretProviderVaultAddr: getEnv("VAULT_ADDR", ""),
		SecretProviderAzure:     getEnvBool("RBTR_SECRET_PROVIDER_AZURE"),
		SSOEnabled:              getEnvBool("RBTR_SSO_ENABLED"),
		SSOIssuer:               getEnv("RBTR_SSO_ISSUER", ""),
		SSOClientID:             getEnv("RBTR_SSO_CLIENT_ID", ""),
		SSOClientSecretRef:      getEnv("RBTR_SSO_CLIENT_SECRET_REF", ""),
		SSORedirectURI:          getEnv("RBTR_SSO_REDIRECT_URI", ""),
		SSOAllowedDomains:       getEnvCSV("RBTR_SSO_ALLOWED_DOMAINS"),
		SSODefaultScopes:        getEnvCSVDefault("RBTR_SSO_DEFAULT_SCOPES", []string{"admin:read", "admin:write"}),
		LicenseKeyPath:          getEnv("RBTR_LICENSE_KEY_PATH", "/etc/rbitr/license.key"),
		LicenseProvider:         getEnv("RBTR_LICENSE_PROVIDER", "self-managed"),
		AWSProductCode:          getEnv("RBTR_AWS_PRODUCT_CODE", ""),
		AWSMarketplaceRegion:    getEnv("RBTR_AWS_REGION", ""),
		AWSCustomerID:           getEnv("RBTR_AWS_CUSTOMER_ID", ""),
		GCPProjectID:            getEnv("RBTR_GCP_PROJECT_ID", ""),
		GCPServiceName:          getEnv("RBTR_GCP_SERVICE_NAME", ""),
		DevAutoTools:            getEnvBool("RBTR_DEV_AUTO_TOOLS"),
		OutboundAllowPrivate:    getEnvBool("RBTR_OUTBOUND_ALLOW_PRIVATE"),
		DevMockInternalURL:      getEnv("RBTR_DEV_MOCK_INTERNAL_URL", "http://localhost:8090"),
		DevJiraURL:              getEnv("RBTR_DEV_JIRA_URL", "http://localhost:8081"),
		SetupTokenRequired:      getEnvBool("RBTR_SETUP_TOKEN_REQUIRED"),
		SetupToken:              getEnv("RBTR_SETUP_TOKEN", ""),
		SetupTokenFile:          getEnv("RBTR_SETUP_TOKEN_FILE", ""),
		SetupAllowedCIDRs:       getEnvCSV("RBTR_SETUP_ALLOWED_CIDRS"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			return parsed
		}
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	return fallback
}

func getEnvDurationFromSeconds(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil && parsed > 0 {
			return time.Duration(parsed) * time.Second
		}
	}
	return fallback
}

func getEnvBool(key string) bool {
	return getEnvBoolDefault(key, false)
}

func getEnvBoolDefault(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			return parsed
		}
	}
	return fallback
}

func getEnvCSVDefault(key string, fallback []string) []string {
	result := getEnvCSV(key)
	if len(result) == 0 {
		return fallback
	}
	return result
}

func getEnvCSV(key string) []string {
	raw := getEnv(key, "")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
