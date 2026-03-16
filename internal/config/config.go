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
	DevAutoTools            bool
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
		FeatureSessionTokens:    getEnvBool("RBTR_FEATURE_SESSION_TOKENS"),
		FeatureFileGovernance:   getEnvBool("RBTR_FEATURE_FILE_GOVERNANCE"),
		FeatureCrossTenantChain: getEnvBool("RBTR_FEATURE_CROSS_TENANT_CHAIN"),
		MaxChainDepth:           getEnvInt("RBTR_MAX_CHAIN_DEPTH", defaultMaxChainDepth),
		SessionTokenTTL:         getEnvDurationFromSeconds("RBTR_SESSION_TOKEN_TTL_SECONDS", defaultSessionTokenTTL),
		DevAutoTools:            getEnvBool("RBTR_DEV_AUTO_TOOLS"),
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
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			return parsed
		}
	}
	return false
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
