package public

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"math"
	"strings"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/store"
)

const (
	rateLimitScopeTenant          = "tenant"
	rateLimitScopeTenantAgent     = "tenant_agent"
	rateLimitScopeTenantTool      = "tenant_tool"
	rateLimitScopeTenantAgentTool = "tenant_agent_tool"
	bucketRateLimit               = 24 * time.Hour
	defaultMinutes                = 60
	defaultDays                   = 10000
)

type rateLimitConfig struct {
	PerMinute int64
	PerDay    int64
	Scope     string
}

type rateLimitKey struct {
	tenantID   string
	agentID    string
	toolID     string
	actionType string
}

type rateLimitViolation struct {
	Window            string `json:"window"`
	Limit             int64  `json:"limit"`
	Remaining         int64  `json:"remaining"`
	RetryAfterSeconds int64  `json:"retry_after_seconds"`
	Scope             string `json:"scope"`
}

type policyRateLimitOverride struct {
	PerMinute *int64
	PerDay    *int64
	Scope     *string
}

//nolint:nilnil // nil violation with nil error means request is allowed or rate limiting is disabled.
func (d *Dependencies) enforceRateLimit(ctx context.Context, tenantID, agentID, toolID, actionType string, constraints map[string]any) (*rateLimitViolation, error) {
	if !d.featureRateLimitingEnabled(ctx) {
		return nil, nil
	}
	if d.Store == nil {
		return nil, nil
	}

	start := time.Now()
	recordLatency := func() {
		if d.Metrics != nil && d.Metrics.RateLimitLatencyMs != nil {
			d.Metrics.RateLimitLatencyMs.Observe(float64(time.Since(start).Milliseconds()))
		}
	}
	defer recordLatency()

	cfg := rateLimitConfig{
		PerMinute: defaultMinutes,
		PerDay:    defaultDays,
		Scope:     rateLimitScopeTenantAgentTool,
	}
	if storedConfig, err := d.Store.GetEffectiveRateLimitConfig(ctx, tenantID); err == nil {
		cfg.PerMinute = storedConfig.PerMinute
		cfg.PerDay = storedConfig.PerDay
		cfg.Scope = storedConfig.Scope
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	applyPolicyRateLimitOverrides(&cfg, parsePolicyRateLimitOverride(constraints))

	now := time.Now().UTC()
	key := applyRateLimitScope(cfg.Scope, rateLimitKey{
		tenantID:   tenantID,
		agentID:    agentID,
		toolID:     toolID,
		actionType: actionType,
	})

	if cfg.PerMinute > 0 {
		allowed, _, err := d.Store.IncrementRateLimitCounter(
			ctx,
			key.tenantID,
			key.agentID,
			key.toolID,
			key.actionType,
			"minute",
			minuteBucketStart(now),
			now,
			cfg.PerMinute,
		)
		if err != nil {
			d.recordRateLimitCheck("error", "minute")
			return nil, err
		}
		if !allowed {
			d.recordRateLimitCheck("exceeded", "minute")
			d.recordRateLimitExceeded("minute", cfg.Scope)
			return &rateLimitViolation{
				Window:            "minute",
				Limit:             cfg.PerMinute,
				Remaining:         0,
				RetryAfterSeconds: secondsUntilNextMinute(now),
				Scope:             cfg.Scope,
			}, nil
		}
		d.recordRateLimitCheck("allowed", "minute")
	}

	if cfg.PerDay > 0 {
		allowed, _, err := d.Store.IncrementRateLimitCounter(
			ctx,
			key.tenantID,
			key.agentID,
			key.toolID,
			key.actionType,
			"day",
			dayBucketStart(now),
			now,
			cfg.PerDay,
		)
		if err != nil {
			d.recordRateLimitCheck("error", "day")
			return nil, err
		}
		if !allowed {
			d.recordRateLimitCheck("exceeded", "day")
			d.recordRateLimitExceeded("day", cfg.Scope)
			return &rateLimitViolation{
				Window:            "day",
				Limit:             cfg.PerDay,
				Remaining:         0,
				RetryAfterSeconds: secondsUntilNextDay(now),
				Scope:             cfg.Scope,
			}, nil
		}
		d.recordRateLimitCheck("allowed", "day")
	}

	return nil, nil
}

func (d *Dependencies) recordRateLimitCheck(result, window string) {
	if d.Metrics == nil || d.Metrics.RateLimitChecksTotal == nil {
		return
	}
	d.Metrics.RateLimitChecksTotal.WithLabelValues(result, window).Inc()
}

func (d *Dependencies) recordRateLimitExceeded(window, scope string) {
	if d.Metrics == nil || d.Metrics.RateLimitExceededTotal == nil {
		return
	}
	d.Metrics.RateLimitExceededTotal.WithLabelValues(window, scope).Inc()
}

func parsePolicyRateLimitOverride(constraints map[string]any) policyRateLimitOverride {
	rawRateLimit, ok := constraints["rate_limit"]
	if !ok {
		return policyRateLimitOverride{}
	}
	rateLimitMap, ok := rawRateLimit.(map[string]any)
	if !ok {
		return policyRateLimitOverride{}
	}

	override := policyRateLimitOverride{}
	if parsed, ok := parseNonNegativeInt(rateLimitMap["per_minute"]); ok {
		override.PerMinute = &parsed
	}
	if parsed, ok := parseNonNegativeInt(rateLimitMap["per_day"]); ok {
		override.PerDay = &parsed
	}
	if scope, ok := normalizeRateLimitScope(rateLimitMap["scope"]); ok {
		override.Scope = &scope
	}
	return override
}

func applyPolicyRateLimitOverrides(cfg *rateLimitConfig, override policyRateLimitOverride) {
	if cfg == nil {
		return
	}
	if override.PerMinute != nil {
		cfg.PerMinute = *override.PerMinute
	}
	if override.PerDay != nil {
		cfg.PerDay = *override.PerDay
	}
	if override.Scope != nil {
		cfg.Scope = *override.Scope
	}
}

func applyRateLimitScope(scope string, key rateLimitKey) rateLimitKey {
	scoped := key
	switch scope {
	case rateLimitScopeTenant:
		scoped.agentID = ""
		scoped.toolID = ""
		scoped.actionType = ""
	case rateLimitScopeTenantAgent:
		scoped.toolID = ""
		scoped.actionType = ""
	case rateLimitScopeTenantTool:
		scoped.agentID = ""
		scoped.actionType = ""
	case rateLimitScopeTenantAgentTool:
		scoped.actionType = ""
	default:
		scoped.actionType = ""
	}
	return scoped
}

func normalizeRateLimitScope(value any) (string, bool) {
	scope, ok := value.(string)
	if !ok {
		return "", false
	}
	trimmed := strings.TrimSpace(scope)
	switch trimmed {
	case rateLimitScopeTenant, rateLimitScopeTenantAgent, rateLimitScopeTenantTool, rateLimitScopeTenantAgentTool:
		return trimmed, true
	default:
		return "", false
	}
}

func parseNonNegativeInt(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		if typed < 0 {
			return 0, false
		}
		return int64(typed), true
	case int64:
		if typed < 0 {
			return 0, false
		}
		return typed, true
	case float64:
		if typed < 0 {
			return 0, false
		}
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil || parsed < 0 {
			return 0, false
		}
		return parsed, true
	}
	return 0, false
}

func minuteBucketStart(now time.Time) time.Time {
	return now.Truncate(time.Minute)
}

func dayBucketStart(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func secondsUntilNextMinute(now time.Time) int64 {
	next := minuteBucketStart(now).Add(time.Minute)
	return int64(math.Ceil(next.Sub(now).Seconds()))
}

func secondsUntilNextDay(now time.Time) int64 {
	next := dayBucketStart(now).Add(bucketRateLimit)
	return int64(math.Ceil(next.Sub(now).Seconds()))
}

func withRateLimitConstraint(constraints map[string]any, violation *rateLimitViolation) map[string]any {
	cloned := map[string]any{}
	maps.Copy(cloned, constraints)
	if violation != nil {
		cloned["rate_limit_enforced"] = map[string]any{
			"window":              violation.Window,
			"limit":               violation.Limit,
			"retry_after_seconds": violation.RetryAfterSeconds,
			"scope":               violation.Scope,
		}
	}
	return cloned
}
