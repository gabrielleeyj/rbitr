// Package compiler turns a structured, form-friendly policy definition into an
// OPA/Rego module compatible with the rbitr policy contract. It lets users set
// permissions without authoring Rego by hand, while keeping OPA as the
// enforcement engine.
package compiler

// Reserved rule identifiers. These are emitted by the compiler as trailing
// fallback rules and are also used by the coverage detector to recognise that
// no explicit tenant rule matched a request. The two packages MUST agree on
// this set, so it is defined once here and consumed elsewhere.
const (
	RuleDefaultDeny         = "rule_default_deny"
	RuleDefaultAllow        = "rule_default_allow"
	RuleDefaultApproval     = "rule_default_require_approval"
	RuleHighRiskUnknown     = "rule_high_risk_unknown"
	RuleCriticalRiskUnknown = "rule_critical_risk_unknown"
)

// FallbackRuleIDs are the rule identifiers that indicate a request was governed
// only by a catch-all rule (i.e. its permissions are ambiguous / unconfigured).
// The coverage detector filters action_decisions on these ids.
func FallbackRuleIDs() []string {
	return []string{RuleDefaultDeny, RuleHighRiskUnknown, RuleCriticalRiskUnknown}
}

const (
	// defaultOutputVersion is the policy output "version" field emitted when the
	// structured policy does not specify one. It matches the value used by the
	// setup wizard's default policy module.
	defaultOutputVersion = "2026-01-20"

	// defaultFallbackRisk is the risk reported by the final default rule, and the
	// fallback used when an incoming request carries no action_risk.
	defaultFallbackRisk = "MEDIUM"

	// approvalTTL bounds (seconds) mirror the gateway's approval expiry limits.
	minApprovalTTLSeconds     = 60
	maxApprovalTTLSeconds     = 86400
	defaultApprovalTTLSeconds = 900

	// maxRulePriority bounds user priorities below the base policy (which uses
	// 1000) so tenant rules never claim base-level precedence in metadata.
	minRulePriority = 0
	maxRulePriority = 999

	// fallbackRulePriority is the metadata priority for the HIGH/CRITICAL risk
	// fallback rules; defaultRulePriority is the terminal default's priority.
	fallbackRulePriority = 80
	defaultRulePriority  = 0
)
