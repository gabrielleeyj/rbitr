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
			logger.Info(
				"request",
				"method", v.Method,
				"uri", v.URI,
				"status", v.Status,
				"latency_ms", v.Latency.Milliseconds(),
				"request_id", getContextString(c, CtxRequestID),
				"tenant_id", getContextString(c, CtxTenantID),
				"agent_id", getContextString(c, CtxAgentID),
				"tool_id", getContextString(c, CtxToolID),
				"action_type", getContextString(c, CtxActionType),
				"decision", getContextString(c, CtxDecision),
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
