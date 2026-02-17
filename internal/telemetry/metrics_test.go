package telemetry

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestNewMetricsRegistersCollectors(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics()
	t.Cleanup(func() {
		prometheus.Unregister(metrics.DecisionsTotal)
		prometheus.Unregister(metrics.GatewayRequests)
		prometheus.Unregister(metrics.ToolExecTotal)
		prometheus.Unregister(metrics.ErrorsTotal)
		prometheus.Unregister(metrics.DecisionLatencyMs)
		prometheus.Unregister(metrics.ToolLatencyMs)
		prometheus.Unregister(metrics.PolicyEvalInvalidTotal)
		prometheus.Unregister(metrics.ApprovalsCreatedTotal)
		prometheus.Unregister(metrics.ApprovalsResolvedTotal)
		prometheus.Unregister(metrics.ApprovalsExecuteTotal)
		prometheus.Unregister(metrics.NotificationsSentTotal)
		prometheus.Unregister(metrics.NotificationsSuppressedTotal)
		prometheus.Unregister(metrics.NotificationsLatencyMs)
		prometheus.Unregister(metrics.TenantAuthFallbackTotal)
		prometheus.Unregister(metrics.RateLimitChecksTotal)
		prometheus.Unregister(metrics.RateLimitExceededTotal)
		prometheus.Unregister(metrics.RateLimitLatencyMs)
	})

	if metrics.DecisionsTotal == nil || metrics.GatewayRequests == nil {
		t.Fatalf("expected metrics to be initialized")
	}

	metrics.DecisionsTotal.WithLabelValues("ALLOW", "TEST").Inc()
	metrics.GatewayRequests.Inc()
	metrics.ToolExecTotal.Inc()
	metrics.ErrorsTotal.Inc()
	metrics.DecisionLatencyMs.Observe(5)
	metrics.ToolLatencyMs.Observe(10)
	metrics.PolicyEvalInvalidTotal.WithLabelValues("schema_violation").Inc()
	metrics.ApprovalsCreatedTotal.Inc()
	metrics.ApprovalsResolvedTotal.WithLabelValues("approved").Inc()
	metrics.ApprovalsExecuteTotal.WithLabelValues("success").Inc()
	metrics.NotificationsSentTotal.WithLabelValues("slack", "APPROVAL.EXPIRING", "sent").Inc()
	metrics.NotificationsSuppressedTotal.WithLabelValues("slack", "APPROVAL.EXPIRING").Inc()
	metrics.NotificationsLatencyMs.WithLabelValues("slack").Observe(5)
	metrics.TenantAuthFallbackTotal.Inc()
	metrics.RateLimitChecksTotal.WithLabelValues("allowed", "minute").Inc()
	metrics.RateLimitExceededTotal.WithLabelValues("minute", "tenant_agent_tool").Inc()
	metrics.RateLimitLatencyMs.Observe(2)

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	expected := map[string]bool{
		"decisions_total":                false,
		"gateway_requests_total":         false,
		"tool_exec_total":                false,
		"errors_total":                   false,
		"decision_latency_ms":            false,
		"tool_latency_ms":                false,
		"policy_eval_invalid_total":      false,
		"approvals_created_total":        false,
		"approvals_resolved_total":       false,
		"approvals_execute_total":        false,
		"notifications_sent_total":       false,
		"notifications_suppressed_total": false,
		"notifications_latency_ms":       false,
		"tenant_auth_fallback_total":     false,
		"rate_limit_checks_total":        false,
		"rate_limit_exceeded_total":      false,
		"rate_limit_latency_ms":          false,
	}
	for _, family := range families {
		if _, ok := expected[family.GetName()]; ok {
			expected[family.GetName()] = true
		}
	}
	for name, found := range expected {
		if !found {
			t.Fatalf("expected metric %s to be registered", name)
		}
	}
}
