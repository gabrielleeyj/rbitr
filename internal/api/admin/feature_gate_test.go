package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/license"
)

func TestFeatureGate_NilValidator_AllowsThrough(t *testing.T) {
	deps := &Dependencies{}

	e := echo.New()
	e.GET("/test", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}, deps.featureGate("approval_workflows"))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestFeatureGate_FeatureEnabled_AllowsThrough(t *testing.T) {
	v := testLicenseProvider(t, "paid", license.Unlimited, license.Unlimited)
	deps := &Dependencies{LicenseProvider: v}

	e := echo.New()
	e.GET("/test", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}, deps.featureGate("approval_workflows"))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestFeatureGate_FeatureDisabled_Returns403(t *testing.T) {
	v := testLicenseProvider(t, "free", 1, 1)
	deps := &Dependencies{LicenseProvider: v}

	e := echo.New()
	e.GET("/test", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}, deps.featureGate("approval_workflows"))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "FEATURE_NOT_AVAILABLE", body["error"])
	assert.Equal(t, "approval_workflows", body["feature"])
	assert.Contains(t, body["message"], "license key")
}

func TestFeatureGate_DifferentFeatures(t *testing.T) {
	v := testLicenseProvider(t, "free", 1, 1)
	deps := &Dependencies{LicenseProvider: v}

	features := []string{"approval_workflows", "evidence_export", "integrations"}
	for _, feature := range features {
		t.Run(feature, func(t *testing.T) {
			e := echo.New()
			e.GET("/test", func(c *echo.Context) error {
				return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
			}, deps.featureGate(feature))

			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", http.NoBody)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusForbidden, rec.Code)
		})
	}
}

func TestHandleEntitlements_NilValidator(t *testing.T) {
	deps := &Dependencies{}

	e := echo.New()
	e.GET("/license/entitlements", deps.handleEntitlements)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/license/entitlements", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "free", body["tier"])

	features, ok := body["features"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, features["approval_workflows"])
	assert.Equal(t, false, features["integrations"])
}

func TestHandleEntitlements_PaidTier(t *testing.T) {
	v := testLicenseProvider(t, "paid", license.Unlimited, license.Unlimited)
	deps := &Dependencies{LicenseProvider: v}

	e := echo.New()
	e.GET("/license/entitlements", deps.handleEntitlements)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/license/entitlements", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "paid", body["tier"])

	features, ok := body["features"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, features["approval_workflows"])
	assert.Equal(t, true, features["evidence_export"])
	assert.Equal(t, true, features["integrations"])
	assert.Equal(t, true, features["custom_policies"])

	limits, ok := body["limits"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(license.Unlimited), limits["max_tenants"])
}

func TestHandleEntitlements_FreeTier(t *testing.T) {
	v := testLicenseProvider(t, "free", 1, 1)
	deps := &Dependencies{LicenseProvider: v}

	e := echo.New()
	e.GET("/license/entitlements", deps.handleEntitlements)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/license/entitlements", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "free", body["tier"])

	features, ok := body["features"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, features["approval_workflows"])
	assert.Equal(t, false, features["integrations"])

	limits, ok := body["limits"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(1), limits["max_tenants"])
	assert.Equal(t, float64(1), limits["max_active_keys"])
}
