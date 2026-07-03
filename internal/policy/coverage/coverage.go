// Package coverage detects endpoints whose gateway permissions are ambiguous —
// governed only by catch-all fallback rules, or never exercised at all — so they
// can be surfaced to operators for explicit configuration.
package coverage

import (
	"context"
	"sort"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/policy/compiler"
)

const (
	// DefaultWindowDays is the default look-back window for observed traffic.
	DefaultWindowDays = 30
	maxWindowDays     = 365
	// DefaultLimit bounds the number of fallback-hit rows returned.
	DefaultLimit = 200
	maxLimit     = 1000

	reasonFallbackHit = "fallback_hit"
	reasonNoTraffic   = "no_traffic"

	hoursPerDay = 24
)

// Reader is the subset of the store the coverage detector needs. Defined here
// (where it is used) to keep the dependency small and testable.
type Reader interface {
	GetTenantConfig(ctx context.Context, tenantID string) (models.TenantConfig, error)
	ListFallbackHitPairs(ctx context.Context, tenantID string, ruleIDs []string, since time.Time, limit int) ([]models.CoverageFallbackHit, error)
	ListUnusedActiveTools(ctx context.Context, tenantID string) ([]string, error)
}

// Gap is a single endpoint (tool, and optionally action) needing configuration.
type Gap struct {
	ToolID                  string     `json:"tool_id"`
	ActionType              string     `json:"action_type,omitempty"`
	ActionRisk              string     `json:"action_risk,omitempty"`
	CurrentFallbackDecision string     `json:"current_fallback_decision,omitempty"`
	FallbackRuleID          string     `json:"fallback_rule_id,omitempty"`
	HitCount                int        `json:"hit_count"`
	LastSeen                *time.Time `json:"last_seen,omitempty"`
	Reason                  string     `json:"reason"` // "fallback_hit" | "no_traffic"
}

// Summary is a rollup of the report.
type Summary struct {
	FallbackHitPairs int `json:"fallback_hit_pairs"`
	UnusedTools      int `json:"unused_tools"`
}

// Report is the full coverage analysis for a tenant.
type Report struct {
	TenantID            string    `json:"tenant_id"`
	ActivePolicyVersion string    `json:"active_policy_version"`
	GeneratedAt         time.Time `json:"generated_at"`
	WindowDays          int       `json:"window_days"`
	Gaps                []Gap     `json:"gaps"`
	Summary             Summary   `json:"summary"`
}

// clampWindowDays keeps the window within sane bounds.
func clampWindowDays(days int) int {
	if days <= 0 {
		return DefaultWindowDays
	}
	if days > maxWindowDays {
		return maxWindowDays
	}
	return days
}

// clampLimit keeps the row limit within sane bounds.
func clampLimit(limit int) int {
	if limit <= 0 {
		return DefaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

// BuildReport assembles the ambiguity report from observed fallback traffic and
// registered-but-unused tools. Results are deterministically ordered.
func BuildReport(ctx context.Context, r Reader, tenantID string, windowDays, limit int) (Report, error) {
	windowDays = clampWindowDays(windowDays)
	limit = clampLimit(limit)
	now := time.Now().UTC()
	since := now.Add(-time.Duration(windowDays) * hoursPerDay * time.Hour)

	report := Report{
		TenantID:    tenantID,
		GeneratedAt: now,
		WindowDays:  windowDays,
		Gaps:        []Gap{},
	}

	if cfg, err := r.GetTenantConfig(ctx, tenantID); err == nil {
		report.ActivePolicyVersion = cfg.ActivePolicyVersion
	}

	hits, err := r.ListFallbackHitPairs(ctx, tenantID, compiler.FallbackRuleIDs(), since, limit)
	if err != nil {
		return Report{}, err
	}
	unused, err := r.ListUnusedActiveTools(ctx, tenantID)
	if err != nil {
		return Report{}, err
	}

	report.Gaps = append(report.Gaps, fallbackGaps(hits)...)
	report.Gaps = append(report.Gaps, noTrafficGaps(unused)...)
	report.Summary = Summary{FallbackHitPairs: len(hits), UnusedTools: len(unused)}

	return report, nil
}

func fallbackGaps(hits []models.CoverageFallbackHit) []Gap {
	gaps := make([]Gap, 0, len(hits))
	for _, h := range hits {
		lastSeen := h.LastSeen
		gaps = append(gaps, Gap{
			ToolID:                  h.ToolID,
			ActionType:              h.ActionType,
			ActionRisk:              h.ActionRisk,
			CurrentFallbackDecision: h.Decision,
			FallbackRuleID:          h.RuleID,
			HitCount:                h.HitCount,
			LastSeen:                &lastSeen,
			Reason:                  reasonFallbackHit,
		})
	}
	sort.SliceStable(gaps, func(i, j int) bool {
		if gaps[i].HitCount != gaps[j].HitCount {
			return gaps[i].HitCount > gaps[j].HitCount
		}
		if gaps[i].ToolID != gaps[j].ToolID {
			return gaps[i].ToolID < gaps[j].ToolID
		}
		return gaps[i].ActionType < gaps[j].ActionType
	})
	return gaps
}

func noTrafficGaps(toolIDs []string) []Gap {
	sorted := append([]string(nil), toolIDs...)
	sort.Strings(sorted)
	gaps := make([]Gap, 0, len(sorted))
	for _, id := range sorted {
		gaps = append(gaps, Gap{ToolID: id, Reason: reasonNoTraffic})
	}
	return gaps
}
