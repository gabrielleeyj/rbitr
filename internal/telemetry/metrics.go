package telemetry

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	DecisionsTotal               *prometheus.CounterVec
	GatewayRequests              prometheus.Counter
	ToolExecTotal                prometheus.Counter
	ErrorsTotal                  prometheus.Counter
	DecisionLatencyMs            prometheus.Histogram
	ToolLatencyMs                prometheus.Histogram
	PolicyEvalInvalidTotal       *prometheus.CounterVec
	ApprovalsCreatedTotal        prometheus.Counter
	ApprovalsResolvedTotal       *prometheus.CounterVec
	ApprovalsExecuteTotal        *prometheus.CounterVec
	NotificationsSentTotal       *prometheus.CounterVec
	NotificationsSuppressedTotal *prometheus.CounterVec
	NotificationsLatencyMs       *prometheus.HistogramVec
	CacheHitsTotal               *prometheus.CounterVec
	CacheMissesTotal             *prometheus.CounterVec
	TenantAuthFallbackTotal      prometheus.Counter
	RateLimitChecksTotal         *prometheus.CounterVec
	RateLimitExceededTotal       *prometheus.CounterVec
	RateLimitLatencyMs           prometheus.Histogram
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
		NotificationsSentTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "notifications_sent_total",
			Help: "Total notifications sent by channel, event type, and result.",
		}, []string{"channel", "event_type", "result"}),
		NotificationsSuppressedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "notifications_suppressed_total",
			Help: "Total notifications suppressed by channel and event type.",
		}, []string{"channel", "event_type"}),
		NotificationsLatencyMs: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "notifications_latency_ms",
			Help:    "Notification delivery latency in ms by channel.",
			Buckets: prometheus.DefBuckets,
		}, []string{"channel"}),
		CacheHitsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "config_cache_hits_total",
			Help: "Total cache hits by cache name.",
		}, []string{"cache"}),
		CacheMissesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "config_cache_misses_total",
			Help: "Total cache misses by cache name.",
		}, []string{"cache"}),
		TenantAuthFallbackTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tenant_auth_fallback_total",
			Help: "Total requests authenticated via deprecated X-Tenant-Key fallback.",
		}),
		RateLimitChecksTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "rate_limit_checks_total",
			Help: "Total rate limit checks by result and window.",
		}, []string{"result", "window"}),
		RateLimitExceededTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "rate_limit_exceeded_total",
			Help: "Total rate limit exceeds by window and scope.",
		}, []string{"window", "scope"}),
		RateLimitLatencyMs: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "rate_limit_latency_ms",
			Help:    "Rate limit evaluation latency in ms.",
			Buckets: prometheus.DefBuckets,
		}),
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
		m.NotificationsSentTotal,
		m.NotificationsSuppressedTotal,
		m.NotificationsLatencyMs,
		m.CacheHitsTotal,
		m.CacheMissesTotal,
		m.TenantAuthFallbackTotal,
		m.RateLimitChecksTotal,
		m.RateLimitExceededTotal,
		m.RateLimitLatencyMs,
	)
	return m
}
