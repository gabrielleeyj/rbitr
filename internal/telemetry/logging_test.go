package telemetry

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestGetContextString(t *testing.T) {
	t.Parallel()

	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	ctx.Set(CtxTenantID, "t1")
	if got := getContextString(ctx, CtxTenantID); got != "t1" {
		t.Fatalf("expected tenant_id t1, got %q", got)
	}

	ctx.Set(CtxTenantID, 123)
	if got := getContextString(ctx, CtxTenantID); got != "" {
		t.Fatalf("expected empty string for non-string context value, got %q", got)
	}
}

func TestRequestLoggerWritesLog(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	e := echo.New()
	e.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	handler := RequestLogger()(func(c *echo.Context) error {
		c.Set(CtxTenantID, "t1")
		c.Set(CtxDecision, "ALLOW")
		return c.NoContent(http.StatusNoContent)
	})

	if err := handler(ctx); err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if !strings.Contains(buf.String(), "request") {
		t.Fatalf("expected log output to contain request, got %q", buf.String())
	}
}
