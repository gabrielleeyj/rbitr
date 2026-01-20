package config

import (
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL   string
	ListenAddr    string
	BodyLimitSize int64
	ResponseLimit int64
}

func Load() Config {
	const defaultLimitBytes = 256 * 1024

	return Config{
		DatabaseURL:   getEnv("DATABASE_URL", "postgres://postgres@localhost:2345/rbitr?sslmode=disable"),
		ListenAddr:    getEnv("LISTEN_ADDR", ":8080"),
		BodyLimitSize: getEnvInt64("BODY_LIMIT_BYTES", defaultLimitBytes),
		ResponseLimit: getEnvInt64("RESPONSE_LIMIT_BYTES", defaultLimitBytes),
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
