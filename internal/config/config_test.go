package config

import "testing"

func TestLoad(t *testing.T) {
	cases := []struct {
		name                   string
		env                    map[string]string
		expectedURL            string
		expectedAddr           string
		expectedBody           int64
		expectedLimit          int64
		expectedArgConstraints bool
	}{
		{
			name: "defaults",
			env: map[string]string{
				"DATABASE_URL":                 "",
				"LISTEN_ADDR":                  "",
				"BODY_LIMIT_BYTES":             "",
				"RESPONSE_LIMIT_BYTES":         "",
				"RBTR_FEATURE_ARG_CONSTRAINTS": "",
			},
			expectedURL:            "postgres://postgres@localhost:2345/rbitr?sslmode=disable",
			expectedAddr:           ":8080",
			expectedBody:           256 * 1024,
			expectedLimit:          256 * 1024,
			expectedArgConstraints: false,
		},
		{
			name: "custom values",
			env: map[string]string{
				"DATABASE_URL":                 "postgres://custom",
				"LISTEN_ADDR":                  ":9090",
				"BODY_LIMIT_BYTES":             "1024",
				"RESPONSE_LIMIT_BYTES":         "2048",
				"RBTR_FEATURE_ARG_CONSTRAINTS": "true",
			},
			expectedURL:            "postgres://custom",
			expectedAddr:           ":9090",
			expectedBody:           1024,
			expectedLimit:          2048,
			expectedArgConstraints: true,
		},
		{
			name: "invalid limits fallback",
			env: map[string]string{
				"DATABASE_URL":                 "",
				"LISTEN_ADDR":                  "",
				"BODY_LIMIT_BYTES":             "nope",
				"RESPONSE_LIMIT_BYTES":         "bad",
				"RBTR_FEATURE_ARG_CONSTRAINTS": "invalid",
			},
			expectedURL:            "postgres://postgres@localhost:2345/rbitr?sslmode=disable",
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
