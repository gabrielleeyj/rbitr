package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/license"
	"github.com/gabrielleeyj/rbitr/internal/store"
)

// --- handleUsageSummary ---

func TestHandleUsageSummary_FreeTier(t *testing.T) {
	v, _ := newProviderWithoutKey(t)
	storeMock := store.NewMockStoreAPI(t)

	period := currentPeriod()
	storeMock.EXPECT().GetTotalUsageForPeriod(mock.Anything, period).Return(int64(4521), nil)
	storeMock.EXPECT().CountTenants(mock.Anything).Return(1, nil)
	storeMock.EXPECT().GetAuditRetentionDays(mock.Anything).Return(7, nil)

	deps := &Dependencies{LicenseProvider: v, Store: storeMock}

	e := echo.New()
	e.GET("/usage", deps.handleUsageSummary)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/usage", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "free", body["tier"])
	assert.Equal(t, period, body["period"])

	usage, ok := body["usage"].(map[string]any)
	require.True(t, ok)

	actions, ok := usage["governed_actions"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(4521), actions["used"])
	assert.Equal(t, float64(10000), actions["limit"])
	assert.InDelta(t, 45.21, actions["pct"], 0.01)

	tenants, ok := usage["tenants"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(1), tenants["used"])
	assert.Equal(t, float64(1), tenants["limit"])
	assert.Equal(t, float64(100), tenants["pct"])

	features, ok := body["features"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, features["approval_workflows"])

	licenseInfo, ok := body["license"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, licenseInfo["valid"])
	assert.Equal(t, "free", licenseInfo["tier"])
}

func TestHandleUsageSummary_PaidTier(t *testing.T) {
	v := testLicenseProvider(t, "paid", license.Unlimited, license.Unlimited)
	storeMock := store.NewMockStoreAPI(t)

	period := currentPeriod()
	storeMock.EXPECT().GetTotalUsageForPeriod(mock.Anything, period).Return(int64(50000), nil)
	storeMock.EXPECT().CountTenants(mock.Anything).Return(5, nil)
	storeMock.EXPECT().GetAuditRetentionDays(mock.Anything).Return(365, nil)

	deps := &Dependencies{LicenseProvider: v, Store: storeMock}

	e := echo.New()
	e.GET("/usage", deps.handleUsageSummary)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/usage", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "paid", body["tier"])

	usage, ok := body["usage"].(map[string]any)
	require.True(t, ok)

	actions, ok := usage["governed_actions"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(50000), actions["used"])
	assert.Equal(t, float64(-1), actions["limit"])
	assert.Equal(t, float64(0), actions["pct"]) // Unlimited = 0%

	features, ok := body["features"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, features["approval_workflows"])
	assert.Equal(t, true, features["integrations"])
}

func TestHandleUsageSummary_NilValidator(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)

	period := currentPeriod()
	storeMock.EXPECT().GetTotalUsageForPeriod(mock.Anything, period).Return(int64(0), nil)
	storeMock.EXPECT().CountTenants(mock.Anything).Return(0, nil)
	storeMock.EXPECT().GetAuditRetentionDays(mock.Anything).Return(365, nil)

	deps := &Dependencies{Store: storeMock}

	e := echo.New()
	e.GET("/usage", deps.handleUsageSummary)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/usage", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "free", body["tier"])

	licenseInfo, ok := body["license"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, licenseInfo["valid"])
}

func TestHandleUsageSummary_NilStore(t *testing.T) {
	deps := &Dependencies{}

	e := echo.New()
	e.GET("/usage", deps.handleUsageSummary)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/usage", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "free", body["tier"])

	usage, ok := body["usage"].(map[string]any)
	require.True(t, ok)
	actions, ok := usage["governed_actions"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(0), actions["used"])
}

// --- handleUsageHistory ---

func TestHandleUsageHistory_DefaultMonths(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.EXPECT().ListAggregatedUsageHistory(mock.Anything, defaultUsageHistoryMonths).Return([]store.PeriodUsageSummary{
		{Period: "2026-03", ActionCount: 5000},
		{Period: "2026-02", ActionCount: 8000},
		{Period: "2026-01", ActionCount: 3000},
	}, nil)

	deps := &Dependencies{Store: storeMock}

	e := echo.New()
	e.GET("/usage/history", deps.handleUsageHistory)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/usage/history", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	periods, ok := body["periods"].([]any)
	require.True(t, ok)
	assert.Len(t, periods, 3)

	first, ok := periods[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "2026-03", first["period"])
	assert.Equal(t, float64(5000), first["action_count"])
	assert.InDelta(t, 50.0, first["pct"], 0.01) // 5000/10000 * 100
}

func TestHandleUsageHistory_CustomMonths(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.EXPECT().ListAggregatedUsageHistory(mock.Anything, 3).Return([]store.PeriodUsageSummary{
		{Period: "2026-03", ActionCount: 1000},
	}, nil)

	deps := &Dependencies{Store: storeMock}

	e := echo.New()
	e.GET("/usage/history", deps.handleUsageHistory)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/usage/history?months=3", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	periods, ok := body["periods"].([]any)
	require.True(t, ok)
	assert.Len(t, periods, 1)
}

func TestHandleUsageHistory_ClampMax(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	// Should be clamped to maxUsageHistoryMonths (24).
	storeMock.EXPECT().ListAggregatedUsageHistory(mock.Anything, maxUsageHistoryMonths).Return(nil, nil)

	deps := &Dependencies{Store: storeMock}

	e := echo.New()
	e.GET("/usage/history", deps.handleUsageHistory)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/usage/history?months=100", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleUsageHistory_EmptyData(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.EXPECT().ListAggregatedUsageHistory(mock.Anything, defaultUsageHistoryMonths).Return(nil, nil)

	deps := &Dependencies{Store: storeMock}

	e := echo.New()
	e.GET("/usage/history", deps.handleUsageHistory)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/usage/history", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	periods, ok := body["periods"].([]any)
	require.True(t, ok)
	assert.Empty(t, periods)
}

func TestHandleUsageHistory_NilStore(t *testing.T) {
	deps := &Dependencies{}

	e := echo.New()
	e.GET("/usage/history", deps.handleUsageHistory)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/usage/history", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	periods, ok := body["periods"].([]any)
	require.True(t, ok)
	assert.Empty(t, periods)
}

func TestHandleUsageHistory_PaidTierUnlimited(t *testing.T) {
	v := testLicenseProvider(t, "paid", license.Unlimited, license.Unlimited)
	storeMock := store.NewMockStoreAPI(t)
	storeMock.EXPECT().ListAggregatedUsageHistory(mock.Anything, defaultUsageHistoryMonths).Return([]store.PeriodUsageSummary{
		{Period: "2026-03", ActionCount: 100000},
	}, nil)

	deps := &Dependencies{LicenseProvider: v, Store: storeMock}

	e := echo.New()
	e.GET("/usage/history", deps.handleUsageHistory)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/usage/history", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	periods, ok := body["periods"].([]any)
	require.True(t, ok)
	require.Len(t, periods, 1)

	first, ok := periods[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(0), first["pct"]) // Unlimited = 0%
}

// --- helper functions ---

func TestBuildGauge(t *testing.T) {
	g := buildGauge(500, 1000)
	assert.Equal(t, int64(500), g.Used)
	assert.Equal(t, int64(1000), g.Limit)
	assert.InDelta(t, 50.0, g.Percent, 0.01)
}

func TestBuildGauge_Unlimited(t *testing.T) {
	g := buildGauge(5000, -1)
	assert.Equal(t, int64(5000), g.Used)
	assert.Equal(t, int64(-1), g.Limit)
	assert.Equal(t, float64(0), g.Percent)
}

func TestCalcPercent_ZeroUsed(t *testing.T) {
	assert.Equal(t, float64(0), calcPercent(0, 10000))
}

func TestCalcPercent_OverLimit(t *testing.T) {
	pct := calcPercent(15000, 10000)
	assert.InDelta(t, 150.0, pct, 0.01)
}
