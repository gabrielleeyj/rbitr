package admin

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/cache"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/testhelpers"
)

func TestInvalidateTenantCaches(t *testing.T) {
	toolCache := cache.New[models.Tool](time.Minute)
	riskCache := cache.New[string](time.Minute)

	toolCache.Set("t1:tool_a", models.Tool{TenantID: "t1", ToolID: "tool_a"})
	riskCache.Set("t1:DATA.EXPORT", "HIGH")
	toolCache.Set("t2:tool_b", models.Tool{TenantID: "t2", ToolID: "tool_b"})
	riskCache.Set("t2:DATA.READ", "LOW")

	deps := Dependencies{
		ToolCache: toolCache,
		RiskCache: riskCache,
	}
	deps.invalidateTenantCaches("t1")

	_, toolFoundT1 := toolCache.Get("t1:tool_a")
	_, riskFoundT1 := riskCache.Get("t1:DATA.EXPORT")
	_, toolFoundT2 := toolCache.Get("t2:tool_b")
	_, riskFoundT2 := riskCache.Get("t2:DATA.READ")

	require.False(t, toolFoundT1)
	require.False(t, riskFoundT1)
	require.True(t, toolFoundT2)
	require.True(t, riskFoundT2)
}

func TestHandleToolConfigUpdateInvalidatesCaches(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
		Return(modelsAdminKey([]string{"admin:write"}), nil)
	storeMock.On("GetTool", context.Background(), "t1", "tool").
		Return(models.Tool{ToolID: "tool", TenantID: "t1", BaseURL: "http://old", AuthType: "bearer", AuthValue: "old"}, nil)
	storeMock.On("UpdateToolConfig", context.Background(), "t1", "tool", "http://example", "bearer", "token", mock.Anything).
		Return(nil)
	storeMock.On("InsertAuditEvent", context.Background(), mock.Anything).Return(nil)

	toolCache := cache.New[models.Tool](time.Minute)
	riskCache := cache.New[string](time.Minute)
	toolCache.Set("t1:tool", models.Tool{TenantID: "t1", ToolID: "tool"})
	riskCache.Set("t1:PAYMENT.REFUND", "HIGH")

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPut,
		testhelpers.MakeBody(ToolConfigRequest{BaseURL: "http://example", AuthType: "bearer", AuthValue: "token"}),
		testhelpers.Params{Names: []string{"tenant_id", "tool_id"}, Values: []string{"t1", "tool"}},
	)
	req.Header.Set(auth.AdminKeyHeader, "key")

	deps := Dependencies{
		Store:     storeMock,
		Metrics:   newTestMetrics(),
		Config:    config.Config{},
		ToolCache: toolCache,
		RiskCache: riskCache,
	}
	err := deps.handleToolConfigUpdate(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, rec.Code)

	_, toolFound := toolCache.Get("t1:tool")
	_, riskFound := riskCache.Get("t1:PAYMENT.REFUND")
	require.False(t, toolFound)
	require.False(t, riskFound)
}
