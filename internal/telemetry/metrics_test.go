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

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	expected := map[string]bool{
		"decisions_total":        false,
		"gateway_requests_total": false,
		"tool_exec_total":        false,
		"errors_total":           false,
		"decision_latency_ms":    false,
		"tool_latency_ms":        false,
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
