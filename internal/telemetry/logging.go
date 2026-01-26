package telemetry

import (
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

const (
	CtxRequestID  = "request_id"
	CtxTenantID   = "tenant_id"
	CtxAgentID    = "agent_id"
	CtxToolID     = "tool_id"
	CtxActionType = "action_type"
	CtxDecision   = "decision"
	CtxAdminID    = "admin_id"
)

func RequestLogger() echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:   true,
		LogLatency:  true,
		LogMethod:   true,
		LogURI:      true,
		HandleError: true,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			logger := c.Logger()
			fields := []any{
				"method", v.Method,
				"uri", v.URI,
				"status", v.Status,
				"latency_ms", v.Latency.Milliseconds(),
			}
			appendIfValue(&fields, "request_id", getContextString(c, CtxRequestID))
			appendIfValue(&fields, "tenant_id", getContextString(c, CtxTenantID))
			appendIfValue(&fields, "agent_id", getContextString(c, CtxAgentID))
			appendIfValue(&fields, "tool_id", getContextString(c, CtxToolID))
			appendIfValue(&fields, "action_type", getContextString(c, CtxActionType))
			appendIfValue(&fields, "decision", getContextString(c, CtxDecision))
			appendIfValue(&fields, "admin_id", getContextString(c, CtxAdminID))
			logger.Info(
				"request",
				fields...,
			)
			return nil
		},
	})
}

func getContextString(c *echo.Context, key string) string {
	if value := c.Get(key); value != nil {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

func appendIfValue(fields *[]any, key, value string) {
	if value == "" {
		return
	}
	*fields = append(*fields, key, value)
}
