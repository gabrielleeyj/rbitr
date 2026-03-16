package policy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/models"
)

func TestEvaluateBasePolicy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		input          map[string]any
		expectedEffect string
	}{
		{
			name:           "low risk ticket create is allowed",
			input:          map[string]any{"action_type": "TICKET.CREATE", "action_risk": "LOW"},
			expectedEffect: "ALLOW",
		},
		{
			name:           "data query is allowed",
			input:          map[string]any{"action_type": "DATA.QUERY", "action_risk": "LOW"},
			expectedEffect: "ALLOW",
		},
		{
			name:           "critical risk requires approval regardless of action type",
			input:          map[string]any{"action_type": "TICKET.CREATE", "action_risk": "CRITICAL"},
			expectedEffect: "REQUIRE_APPROVAL",
		},
		{
			name:           "data delete at high risk requires approval",
			input:          map[string]any{"action_type": "DATA.DELETE", "action_risk": "HIGH"},
			expectedEffect: "REQUIRE_APPROVAL",
		},
		{
			name:           "data delete at critical risk requires approval",
			input:          map[string]any{"action_type": "DATA.DELETE", "action_risk": "CRITICAL"},
			expectedEffect: "REQUIRE_APPROVAL",
		},
		{
			name:           "data delete at low risk is allowed",
			input:          map[string]any{"action_type": "DATA.DELETE", "action_risk": "LOW"},
			expectedEffect: "ALLOW",
		},
		{
			name:           "access grant at any risk requires approval",
			input:          map[string]any{"action_type": "ACCESS.GRANT", "action_risk": "LOW"},
			expectedEffect: "REQUIRE_APPROVAL",
		},
		{
			name:           "access role change at any risk requires approval",
			input:          map[string]any{"action_type": "ACCESS.ROLE_CHANGE", "action_risk": "MEDIUM"},
			expectedEffect: "REQUIRE_APPROVAL",
		},
		{
			name:           "data export at high risk requires approval",
			input:          map[string]any{"action_type": "DATA.EXPORT", "action_risk": "HIGH"},
			expectedEffect: "REQUIRE_APPROVAL",
		},
		{
			name:           "data bulk export at critical risk requires approval",
			input:          map[string]any{"action_type": "DATA.BULK_EXPORT", "action_risk": "CRITICAL"},
			expectedEffect: "REQUIRE_APPROVAL",
		},
		{
			name:           "crm delete at high risk requires approval",
			input:          map[string]any{"action_type": "CRM.DELETE", "action_risk": "HIGH"},
			expectedEffect: "REQUIRE_APPROVAL",
		},
		{
			name:           "medium risk ticket update is allowed",
			input:          map[string]any{"action_type": "TICKET.UPDATE", "action_risk": "MEDIUM"},
			expectedEffect: "ALLOW",
		},
		{
			name:           "empty input is allowed",
			input:          map[string]any{},
			expectedEffect: "ALLOW",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := EvaluateBasePolicy(context.Background(), tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.expectedEffect, result.Effect, "effect mismatch for input: %v", tc.input)
		})
	}
}

func TestMergeBasePolicyDecision(t *testing.T) {
	t.Parallel()

	tenantAllow := Result{
		Version:       "2026-01-20",
		Decision:      "ALLOW",
		Risk:          "LOW",
		Rule:          models.DecisionRule{ID: "rule_allow", Priority: 10},
		Reasons:       []models.DecisionReason{{Code: "ALLOW", Message: "ok"}},
		Constraints:   map[string]any{},
		Tags:          []string{},
		PolicyVersion: "p_v1",
	}

	tenantDeny := Result{
		Version:       "2026-01-20",
		Decision:      "DENY",
		Risk:          "HIGH",
		Rule:          models.DecisionRule{ID: "rule_deny", Priority: 100},
		Reasons:       []models.DecisionReason{{Code: "DENY", Message: "denied"}},
		Constraints:   map[string]any{},
		Tags:          []string{},
		PolicyVersion: "p_v1",
	}

	tenantRequireApproval := Result{
		Version:       "2026-01-20",
		Decision:      "REQUIRE_APPROVAL",
		Risk:          "MEDIUM",
		Rule:          models.DecisionRule{ID: "rule_approval", Priority: 50},
		Reasons:       []models.DecisionReason{{Code: "APPROVAL", Message: "needs approval"}},
		Constraints:   map[string]any{},
		Tags:          []string{},
		PolicyVersion: "p_v1",
	}

	cases := []struct {
		name             string
		base             BasePolicyResult
		tenant           Result
		expectedDecision string
		expectedTag      string
	}{
		{
			name:             "base DENY overrides tenant ALLOW",
			base:             BasePolicyResult{Effect: "DENY", RuleID: "base_deny", Reason: "base denied"},
			tenant:           tenantAllow,
			expectedDecision: "DENY",
			expectedTag:      "base_policy:DENY",
		},
		{
			name:             "base DENY overrides tenant REQUIRE_APPROVAL",
			base:             BasePolicyResult{Effect: "DENY", RuleID: "base_deny", Reason: "base denied"},
			tenant:           tenantRequireApproval,
			expectedDecision: "DENY",
			expectedTag:      "base_policy:DENY",
		},
		{
			name:             "base REQUIRE_APPROVAL upgrades tenant ALLOW",
			base:             BasePolicyResult{Effect: "REQUIRE_APPROVAL", RuleID: "base_require", Reason: "base requires approval"},
			tenant:           tenantAllow,
			expectedDecision: "REQUIRE_APPROVAL",
			expectedTag:      "base_policy:REQUIRE_APPROVAL",
		},
		{
			name:             "base REQUIRE_APPROVAL keeps tenant DENY",
			base:             BasePolicyResult{Effect: "REQUIRE_APPROVAL", RuleID: "base_require", Reason: "base requires approval"},
			tenant:           tenantDeny,
			expectedDecision: "DENY",
			expectedTag:      "base_policy:REQUIRE_APPROVAL",
		},
		{
			name:             "base REQUIRE_APPROVAL keeps tenant REQUIRE_APPROVAL",
			base:             BasePolicyResult{Effect: "REQUIRE_APPROVAL", RuleID: "base_require", Reason: "base requires approval"},
			tenant:           tenantRequireApproval,
			expectedDecision: "REQUIRE_APPROVAL",
			expectedTag:      "base_policy:REQUIRE_APPROVAL",
		},
		{
			name:             "base ALLOW defers to tenant ALLOW",
			base:             BasePolicyResult{Effect: "ALLOW", RuleID: "base_allow", Reason: "base allows"},
			tenant:           tenantAllow,
			expectedDecision: "ALLOW",
			expectedTag:      "base_policy:ALLOW",
		},
		{
			name:             "base ALLOW defers to tenant DENY",
			base:             BasePolicyResult{Effect: "ALLOW", RuleID: "base_allow", Reason: "base allows"},
			tenant:           tenantDeny,
			expectedDecision: "DENY",
			expectedTag:      "base_policy:ALLOW",
		},
		{
			name:             "base ALLOW defers to tenant REQUIRE_APPROVAL",
			base:             BasePolicyResult{Effect: "ALLOW", RuleID: "base_allow", Reason: "base allows"},
			tenant:           tenantRequireApproval,
			expectedDecision: "REQUIRE_APPROVAL",
			expectedTag:      "base_policy:ALLOW",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			merged := MergeBasePolicyDecision(tc.base, &tc.tenant)
			require.Equal(t, tc.expectedDecision, merged.Decision)
			require.Contains(t, merged.Tags, tc.expectedTag)
		})
	}
}

func TestAppendTag(t *testing.T) {
	t.Parallel()

	original := []string{"a", "b"}
	result := appendTag(original, "c")
	require.Equal(t, []string{"a", "b", "c"}, result)
	// Verify original is not mutated.
	require.Equal(t, []string{"a", "b"}, original)
}

func TestAppendTagNil(t *testing.T) {
	t.Parallel()

	result := appendTag(nil, "c")
	require.Equal(t, []string{"c"}, result)
}
