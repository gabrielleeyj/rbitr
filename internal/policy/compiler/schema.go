package compiler

// Effect is the decision a rule produces when it matches.
type Effect string

const (
	EffectAllow           Effect = "ALLOW"
	EffectDeny            Effect = "DENY"
	EffectRequireApproval Effect = "REQUIRE_APPROVAL"
)

// Matcher selects requests. Fields are AND-ed together; values within a single
// field are OR-ed (set membership). An empty field matches anything, so an
// entirely empty Matcher is an unconditional catch-all.
type Matcher struct {
	ToolIDs     []string `json:"tool_ids,omitempty"`
	ActionTypes []string `json:"action_types,omitempty"` // exact, or prefix form "MCP.*"
	ActionRisks []string `json:"action_risks,omitempty"` // LOW|MEDIUM|HIGH|CRITICAL
}

// IsEmpty reports whether the matcher constrains nothing (catch-all).
func (m Matcher) IsEmpty() bool {
	return len(m.ToolIDs) == 0 && len(m.ActionTypes) == 0 && len(m.ActionRisks) == 0
}

// ApprovalConstraint configures the approval requirement emitted for a
// REQUIRE_APPROVAL rule.
type ApprovalConstraint struct {
	ExpiresInSeconds int    `json:"expires_in_seconds,omitempty"`
	Reason           string `json:"reason,omitempty"`
}

// Rule is a single permission entry authored through the structured builder.
type Rule struct {
	ID       string              `json:"id"`       // ^[a-z0-9_]{1,64}$, unique, not reserved
	Priority int                 `json:"priority"` // higher = evaluated first (0..999)
	Effect   Effect              `json:"effect"`
	Match    Matcher             `json:"match"`
	Reason   string              `json:"reason,omitempty"`
	Approval *ApprovalConstraint `json:"approval,omitempty"` // only for REQUIRE_APPROVAL
}

// StructuredPolicy is the full, serialisable policy the UI edits. It compiles to
// a Rego module that is stored and enforced exactly like a hand-written one.
type StructuredPolicy struct {
	SchemaVersion       string `json:"schema_version"` // model version, e.g. "1"
	OutputVersion       string `json:"output_version"` // rego "version" field
	DefaultEffect       Effect `json:"default_effect"` // fallback when no rule matches
	Rules               []Rule `json:"rules"`
	AppendRiskFallbacks *bool  `json:"append_risk_fallbacks,omitempty"` // default true
}

// appendRiskFallbacks reports whether the standard HIGH/CRITICAL risk fallbacks
// should be emitted. Defaults to true when unset.
func (p *StructuredPolicy) appendRiskFallbacks() bool {
	return p.AppendRiskFallbacks == nil || *p.AppendRiskFallbacks
}

// outputVersion returns the configured output version or the default.
func (p *StructuredPolicy) outputVersion() string {
	if p.OutputVersion == "" {
		return defaultOutputVersion
	}
	return p.OutputVersion
}
