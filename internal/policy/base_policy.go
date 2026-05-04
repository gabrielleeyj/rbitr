package policy

import (
	"context"
	"sync"

	"github.com/open-policy-agent/opa/v1/rego"

	"github.com/gabrielleeyj/rbitr/internal/opa"
)

// basePolicyModule is the system-level Rego policy that is always evaluated
// before any tenant policy. Its decisions cannot be overridden by tenant policies:
//   - Base DENY → final DENY (tenant policy never evaluated)
//   - Base REQUIRE_APPROVAL → tenant cannot downgrade to ALLOW
//   - Base ALLOW → tenant policy decision is used
const basePolicyModule = `package rbitr.base_policy

import rego.v1

# Actions that always require approval regardless of tenant policy.
critical_risk_actions := {
	"DATA.DELETE",
	"CRM.DELETE",
	"DATA.BULK_EXPORT",
	"DATA.EXPORT",
	"ACCESS.GRANT",
	"ACCESS.ROLE_CHANGE"
}

# CRITICAL risk actions always require approval.
base_require_approval if {
	input.action_risk == "CRITICAL"
}

# Destructive and access actions require approval at HIGH or CRITICAL risk.
base_require_approval if {
	critical_risk_actions[input.action_type]
	input.action_risk in {"HIGH", "CRITICAL"}
}

# Identity/access mutations always require approval at any risk level.
base_require_approval if {
	input.action_type in {"ACCESS.GRANT", "ACCESS.ROLE_CHANGE"}
}

decision := {
	"effect": "REQUIRE_APPROVAL",
	"rule_id": "base_require_approval",
	"reason": "Base policy: action requires approval"
} if {
	base_require_approval
} else := {
	"effect": "ALLOW",
	"rule_id": "base_allow",
	"reason": "Base policy: no restriction"
}
`

const (
	basePolicyEffectAllow           = "ALLOW"
	basePolicyEffectDeny            = "DENY"
	basePolicyEffectRequireApproval = "REQUIRE_APPROVAL"

	basePolicyRuleAllow = "base_allow"
)

// BasePolicyResult holds the outcome of a base policy evaluation.
type BasePolicyResult struct {
	Effect string // "ALLOW", "DENY", or "REQUIRE_APPROVAL"
	RuleID string
	Reason string
}

type basePolicyEvaluator struct {
	mu       sync.Mutex
	prepared rego.PreparedEvalQuery
	ready    bool
}

var globalBasePolicy basePolicyEvaluator //nolint:gochecknoglobals // singleton for perf; prepared query is thread-safe.

// EvaluateBasePolicy evaluates the system-level base policy against the given input.
// The base policy runs the same input as the tenant policy.
func EvaluateBasePolicy(ctx context.Context, input map[string]any) (BasePolicyResult, error) {
	prepared, err := getBasePolicyPrepared(ctx)
	if err != nil {
		return BasePolicyResult{}, err
	}

	resultSet, err := prepared.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return BasePolicyResult{}, err
	}

	return parseBasePolicyResult(resultSet)
}

func getBasePolicyPrepared(ctx context.Context) (rego.PreparedEvalQuery, error) {
	globalBasePolicy.mu.Lock()
	defer globalBasePolicy.mu.Unlock()

	if globalBasePolicy.ready {
		return globalBasePolicy.prepared, nil
	}

	r := rego.New(
		rego.Query("data.rbitr.base_policy.decision"),
		rego.Module("base_policy.rego", basePolicyModule),
	)
	prepared, err := r.PrepareForEval(ctx)
	if err != nil {
		return rego.PreparedEvalQuery{}, err
	}

	globalBasePolicy.prepared = prepared
	globalBasePolicy.ready = true
	return prepared, nil
}

func parseBasePolicyResult(resultSet rego.ResultSet) (BasePolicyResult, error) {
	if len(resultSet) == 0 || len(resultSet[0].Expressions) == 0 {
		// No result means no restriction from base policy.
		return BasePolicyResult{Effect: basePolicyEffectAllow, RuleID: basePolicyRuleAllow, Reason: "Base policy: no result"}, nil
	}

	value, ok := resultSet[0].Expressions[0].Value.(map[string]any)
	if !ok {
		return BasePolicyResult{Effect: basePolicyEffectAllow, RuleID: basePolicyRuleAllow, Reason: "Base policy: parse error"}, nil
	}

	effect, _ := value["effect"].(string)
	ruleID, _ := value["rule_id"].(string)
	reason, _ := value["reason"].(string)

	if effect == "" {
		effect = basePolicyEffectAllow
	}
	if !opa.IsValidDecision(effect) {
		effect = basePolicyEffectAllow
	}

	return BasePolicyResult{
		Effect: effect,
		RuleID: ruleID,
		Reason: reason,
	}, nil
}

// MergeBasePolicyDecision applies the base policy constraint to a tenant policy result.
// The merge follows these rules:
//   - Base DENY → final DENY (tenant decision ignored)
//   - Base REQUIRE_APPROVAL + Tenant ALLOW → final REQUIRE_APPROVAL (base wins)
//   - Base REQUIRE_APPROVAL + Tenant DENY → final DENY (strictest wins)
//   - Base REQUIRE_APPROVAL + Tenant REQUIRE_APPROVAL → final REQUIRE_APPROVAL
//   - Base ALLOW → tenant decision used as-is
func MergeBasePolicyDecision(base BasePolicyResult, tenant *Result) Result {
	switch base.Effect {
	case basePolicyEffectDeny:
		return Result{
			Version:       tenant.Version,
			Decision:      basePolicyEffectDeny,
			Risk:          tenant.Risk,
			Rule:          tenant.Rule,
			Reasons:       tenant.Reasons,
			Constraints:   tenant.Constraints,
			Tags:          appendTag(tenant.Tags, tagBasePolicyDeny),
			MatchedRules:  tenant.MatchedRules,
			PolicyVersion: tenant.PolicyVersion,
		}

	case basePolicyEffectRequireApproval:
		decision := tenant.Decision
		if decision == basePolicyEffectAllow {
			decision = basePolicyEffectRequireApproval
		}
		return Result{
			Version:       tenant.Version,
			Decision:      decision,
			Risk:          tenant.Risk,
			Rule:          tenant.Rule,
			Reasons:       tenant.Reasons,
			Constraints:   tenant.Constraints,
			Tags:          appendTag(tenant.Tags, "base_policy:REQUIRE_APPROVAL"),
			MatchedRules:  tenant.MatchedRules,
			PolicyVersion: tenant.PolicyVersion,
		}

	default:
		return Result{
			Version:       tenant.Version,
			Decision:      tenant.Decision,
			Risk:          tenant.Risk,
			Rule:          tenant.Rule,
			Reasons:       tenant.Reasons,
			Constraints:   tenant.Constraints,
			Tags:          appendTag(tenant.Tags, "base_policy:ALLOW"),
			MatchedRules:  tenant.MatchedRules,
			PolicyVersion: tenant.PolicyVersion,
		}
	}
}

func appendTag(tags []string, tag string) []string {
	out := make([]string, len(tags), len(tags)+1)
	copy(out, tags)
	return append(out, tag)
}
