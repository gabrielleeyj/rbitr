package telemetry

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	DecisionsTotal         *prometheus.CounterVec
	GatewayRequests        prometheus.Counter
	ToolExecTotal          prometheus.Counter
	ErrorsTotal            prometheus.Counter
	DecisionLatencyMs      prometheus.Histogram
	ToolLatencyMs          prometheus.Histogram
	PolicyEvalInvalidTotal *prometheus.CounterVec
	ApprovalsCreatedTotal  prometheus.Counter
	ApprovalsResolvedTotal *prometheus.CounterVec
	ApprovalsExecuteTotal  *prometheus.CounterVec
}

func NewMetrics() *Metrics {
	m := &Metrics{
		DecisionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "decisions_total",
			Help: "Total decisions by decision and action type.",
		}, []string{"decision", "action_type"}),
		GatewayRequests: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gateway_requests_total",
			Help: "Total gateway requests.",
		}),
		ToolExecTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tool_exec_total",
			Help: "Total tool executions.",
		}),
		ErrorsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "errors_total",
			Help: "Total gateway errors.",
		}),
		DecisionLatencyMs: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "decision_latency_ms",
			Help:    "Decision latency in ms.",
			Buckets: prometheus.DefBuckets,
		}),
		ToolLatencyMs: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "tool_latency_ms",
			Help:    "Tool execution latency in ms.",
			Buckets: prometheus.DefBuckets,
		}),
		PolicyEvalInvalidTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "policy_eval_invalid_total",
			Help: "Total invalid policy outputs by reason.",
		}, []string{"reason"}),
		ApprovalsCreatedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "approvals_created_total",
			Help: "Total approval requests created.",
		}),
		ApprovalsResolvedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "approvals_resolved_total",
			Help: "Total approvals resolved by resolution.",
		}, []string{"resolution"}),
		ApprovalsExecuteTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "approvals_execute_total",
			Help: "Total approval executions by result.",
		}, []string{"result"}),
	}
	prometheus.MustRegister(
		m.DecisionsTotal,
		m.GatewayRequests,
		m.ToolExecTotal,
		m.ErrorsTotal,
		m.DecisionLatencyMs,
		m.ToolLatencyMs,
		m.PolicyEvalInvalidTotal,
		m.ApprovalsCreatedTotal,
		m.ApprovalsResolvedTotal,
		m.ApprovalsExecuteTotal,
	)
	return m
}
