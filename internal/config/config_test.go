package config

import (
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	cases := []struct {
		name                   string
		env                    map[string]string
		expectedURL            string
		expectedMaxOpen        int
		expectedMaxIdle        int
		expectedConnLifetime   time.Duration
		expectedConnIdleTime   time.Duration
		expectedAddr           string
		expectedBody           int64
		expectedLimit          int64
		expectedArgConstraints bool
	}{
		{
			name: "defaults",
			env: map[string]string{
				"DATABASE_URL":                  "",
				"DB_MAX_OPEN_CONNS":             "",
				"DB_MAX_IDLE_CONNS":             "",
				"DB_CONN_MAX_LIFETIME_SECONDS":  "",
				"DB_CONN_MAX_IDLE_TIME_SECONDS": "",
				"LISTEN_ADDR":                   "",
				"BODY_LIMIT_BYTES":              "",
				"RESPONSE_LIMIT_BYTES":          "",
				"RBTR_FEATURE_ARG_CONSTRAINTS":  "",
			},
			expectedURL:            "postgres://postgres@localhost:2345/rbitr?sslmode=require",
			expectedMaxOpen:        30,
			expectedMaxIdle:        10,
			expectedConnLifetime:   30 * time.Minute,
			expectedConnIdleTime:   5 * time.Minute,
			expectedAddr:           ":8080",
			expectedBody:           256 * 1024,
			expectedLimit:          256 * 1024,
			expectedArgConstraints: false,
		},
		{
			name: "custom values",
			env: map[string]string{
				"DATABASE_URL":                  "postgres://custom",
				"DB_MAX_OPEN_CONNS":             "64",
				"DB_MAX_IDLE_CONNS":             "16",
				"DB_CONN_MAX_LIFETIME_SECONDS":  "3600",
				"DB_CONN_MAX_IDLE_TIME_SECONDS": "120",
				"LISTEN_ADDR":                   ":9090",
				"BODY_LIMIT_BYTES":              "1024",
				"RESPONSE_LIMIT_BYTES":          "2048",
				"RBTR_FEATURE_ARG_CONSTRAINTS":  "true",
			},
			expectedURL:            "postgres://custom",
			expectedMaxOpen:        64,
			expectedMaxIdle:        16,
			expectedConnLifetime:   1 * time.Hour,
			expectedConnIdleTime:   2 * time.Minute,
			expectedAddr:           ":9090",
			expectedBody:           1024,
			expectedLimit:          2048,
			expectedArgConstraints: true,
		},
		{
			name: "invalid limits fallback",
			env: map[string]string{
				"DATABASE_URL":                  "",
				"DB_MAX_OPEN_CONNS":             "bad",
				"DB_MAX_IDLE_CONNS":             "nope",
				"DB_CONN_MAX_LIFETIME_SECONDS":  "0",
				"DB_CONN_MAX_IDLE_TIME_SECONDS": "invalid",
				"LISTEN_ADDR":                   "",
				"BODY_LIMIT_BYTES":              "nope",
				"RESPONSE_LIMIT_BYTES":          "bad",
				"RBTR_FEATURE_ARG_CONSTRAINTS":  "invalid",
			},
			expectedURL:            "postgres://postgres@localhost:2345/rbitr?sslmode=require",
			expectedMaxOpen:        30,
			expectedMaxIdle:        10,
			expectedConnLifetime:   30 * time.Minute,
			expectedConnIdleTime:   5 * time.Minute,
			expectedAddr:           ":8080",
			expectedBody:           256 * 1024,
			expectedLimit:          256 * 1024,
			expectedArgConstraints: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			cfg := Load()
			if cfg.DatabaseURL != tc.expectedURL {
				t.Fatalf("expected DatabaseURL %q got %q", tc.expectedURL, cfg.DatabaseURL)
			}
			if cfg.DBMaxOpenConns != tc.expectedMaxOpen {
				t.Fatalf("expected DBMaxOpenConns %d got %d", tc.expectedMaxOpen, cfg.DBMaxOpenConns)
			}
			if cfg.DBMaxIdleConns != tc.expectedMaxIdle {
				t.Fatalf("expected DBMaxIdleConns %d got %d", tc.expectedMaxIdle, cfg.DBMaxIdleConns)
			}
			if cfg.DBConnMaxLifetime != tc.expectedConnLifetime {
				t.Fatalf("expected DBConnMaxLifetime %s got %s", tc.expectedConnLifetime, cfg.DBConnMaxLifetime)
			}
			if cfg.DBConnMaxIdleTime != tc.expectedConnIdleTime {
				t.Fatalf("expected DBConnMaxIdleTime %s got %s", tc.expectedConnIdleTime, cfg.DBConnMaxIdleTime)
			}
			if cfg.ListenAddr != tc.expectedAddr {
				t.Fatalf("expected ListenAddr %q got %q", tc.expectedAddr, cfg.ListenAddr)
			}
			if cfg.BodyLimitSize != tc.expectedBody {
				t.Fatalf("expected BodyLimitSize %d got %d", tc.expectedBody, cfg.BodyLimitSize)
			}
			if cfg.ResponseLimit != tc.expectedLimit {
				t.Fatalf("expected ResponseLimit %d got %d", tc.expectedLimit, cfg.ResponseLimit)
			}
			if cfg.FeatureArgConstraints != tc.expectedArgConstraints {
				t.Fatalf("expected FeatureArgConstraints %t got %t", tc.expectedArgConstraints, cfg.FeatureArgConstraints)
			}
		})
	}
}
