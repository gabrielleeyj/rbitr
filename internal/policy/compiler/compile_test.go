package compiler

import (
	"context"
	"testing"

	"github.com/gabrielleeyj/rbitr/internal/opa"
)

func ptrBool(b bool) *bool { return &b }

// mustCompile compiles a policy and fails the test on error.
func mustCompile(t *testing.T, p *StructuredPolicy) string {
	t.Helper()
	module, err := Compile(p)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	return module
}

// evaluate compiles+evaluates a policy against an input and returns the result.
func evaluate(t *testing.T, module string, input map[string]any) opa.Result {
	t.Helper()
	res, err := opa.NewEngine(module).Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate() error = %v\nmodule:\n%s", err, module)
	}
	return res
}

func samplePolicy() *StructuredPolicy {
	return &StructuredPolicy{
		SchemaVersion: "1",
		DefaultEffect: EffectDeny,
		Rules: []Rule{
			{
				ID:       "allow_reads",
				Priority: 100,
				Effect:   EffectAllow,
				Match:    Matcher{ActionTypes: []string{"DATA.READ", "DATA.QUERY", "CRM.READ"}},
			},
			{
				ID:       "refund_approval",
				Priority: 90,
				Effect:   EffectRequireApproval,
				Match:    Matcher{ToolIDs: []string{"mock_internal"}, ActionTypes: []string{"PAYMENT.REFUND"}},
				Approval: &ApprovalConstraint{ExpiresInSeconds: 900, Reason: "High-value transaction"},
			},
			{
				ID:       "deny_bulk_export",
				Priority: 80,
				Effect:   EffectDeny,
				Match:    Matcher{ActionTypes: []string{"DATA.BULK_EXPORT", "DATA.EXPORT"}},
			},
		},
	}
}

func TestCompileProducesValidRego(t *testing.T) {
	// Arrange
	module := mustCompile(t, samplePolicy())

	// Act
	_, err := opa.PrepareQuery(context.Background(), module)
	// Assert
	if err != nil {
		t.Fatalf("PrepareQuery() error = %v\nmodule:\n%s", err, module)
	}
}

func TestCompileDecisionMatrix(t *testing.T) {
	module := mustCompile(t, samplePolicy())

	tests := []struct {
		name         string
		input        map[string]any
		wantDecision string
		wantRuleID   string
	}{
		{
			name:         "allow read",
			input:        map[string]any{"action_type": "DATA.READ", "action_risk": "LOW"},
			wantDecision: opa.DecisionAllow,
			wantRuleID:   "allow_reads",
		},
		{
			name:         "refund on matching tool requires approval",
			input:        map[string]any{"tool_id": "mock_internal", "action_type": "PAYMENT.REFUND", "action_risk": "HIGH"},
			wantDecision: opa.DecisionRequireApproval,
			wantRuleID:   "refund_approval",
		},
		{
			name:         "refund on other tool falls through to risk fallback",
			input:        map[string]any{"tool_id": "other", "action_type": "PAYMENT.REFUND", "action_risk": "HIGH"},
			wantDecision: opa.DecisionRequireApproval,
			wantRuleID:   RuleHighRiskUnknown,
		},
		{
			name:         "bulk export denied",
			input:        map[string]any{"action_type": "DATA.BULK_EXPORT", "action_risk": "CRITICAL"},
			wantDecision: opa.DecisionDeny,
			wantRuleID:   "deny_bulk_export",
		},
		{
			name:         "unknown low-risk action hits default deny",
			input:        map[string]any{"action_type": "DATA.UPDATE", "action_risk": "LOW"},
			wantDecision: opa.DecisionDeny,
			wantRuleID:   RuleDefaultDeny,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := evaluate(t, module, tt.input)
			if res.Decision != tt.wantDecision {
				t.Errorf("decision = %q, want %q", res.Decision, tt.wantDecision)
			}
			if res.Rule.ID != tt.wantRuleID {
				t.Errorf("rule.id = %q, want %q", res.Rule.ID, tt.wantRuleID)
			}
		})
	}
}

func TestCompileApprovalConstraints(t *testing.T) {
	module := mustCompile(t, samplePolicy())

	res := evaluate(t, module, map[string]any{
		"tool_id": "mock_internal", "action_type": "PAYMENT.REFUND", "action_risk": "HIGH",
	})

	approval, ok := res.Constraints["approval"].(map[string]any)
	if !ok {
		t.Fatalf("expected approval constraint, got %#v", res.Constraints)
	}
	if reason, _ := approval["reason"].(string); reason != "High-value transaction" {
		t.Errorf("approval.reason = %q, want %q", reason, "High-value transaction")
	}
}

func TestCompilePriorityOrderingWins(t *testing.T) {
	// A lower-priority DENY and higher-priority ALLOW on the same action: ALLOW wins.
	p := StructuredPolicy{
		DefaultEffect: EffectDeny,
		Rules: []Rule{
			{ID: "deny_it", Priority: 10, Effect: EffectDeny, Match: Matcher{ActionTypes: []string{"DATA.READ"}}},
			{ID: "allow_it", Priority: 99, Effect: EffectAllow, Match: Matcher{ActionTypes: []string{"DATA.READ"}}},
		},
	}
	module := mustCompile(t, &p)

	res := evaluate(t, module, map[string]any{"action_type": "DATA.READ", "action_risk": "LOW"})
	if res.Rule.ID != "allow_it" {
		t.Errorf("rule.id = %q, want allow_it (higher priority)", res.Rule.ID)
	}
}

func TestCompileMCPPrefixMatch(t *testing.T) {
	p := StructuredPolicy{
		DefaultEffect: EffectDeny,
		Rules: []Rule{
			{ID: "allow_mcp", Priority: 50, Effect: EffectAllow, Match: Matcher{ActionTypes: []string{"MCP.*"}}},
		},
	}
	module := mustCompile(t, &p)

	res := evaluate(t, module, map[string]any{"action_type": "MCP.SEND_EMAIL", "action_risk": "LOW"})
	if res.Decision != opa.DecisionAllow || res.Rule.ID != "allow_mcp" {
		t.Errorf("got decision=%q rule=%q, want ALLOW/allow_mcp", res.Decision, res.Rule.ID)
	}
}

func TestCompileMixedExactAndPrefix(t *testing.T) {
	p := StructuredPolicy{
		DefaultEffect: EffectDeny,
		Rules: []Rule{
			{ID: "mixed", Priority: 50, Effect: EffectAllow, Match: Matcher{ActionTypes: []string{"DATA.READ", "MCP.*"}}},
		},
	}
	module := mustCompile(t, &p)

	for _, at := range []string{"DATA.READ", "MCP.FOO"} {
		res := evaluate(t, module, map[string]any{"action_type": at, "action_risk": "LOW"})
		if res.Rule.ID != "mixed" {
			t.Errorf("action_type=%q rule.id=%q, want mixed", at, res.Rule.ID)
		}
	}
	// Non-matching action falls through.
	res := evaluate(t, module, map[string]any{"action_type": "CRM.READ", "action_risk": "LOW"})
	if res.Rule.ID != RuleDefaultDeny {
		t.Errorf("rule.id = %q, want default deny", res.Rule.ID)
	}
}

func TestCompileToolOnlyRuleWithMissingRisk(t *testing.T) {
	p := StructuredPolicy{
		DefaultEffect:       EffectDeny,
		AppendRiskFallbacks: ptrBool(false),
		Rules: []Rule{
			{ID: "trust_tool", Priority: 50, Effect: EffectAllow, Match: Matcher{ToolIDs: []string{"safe_tool"}}},
		},
	}
	module := mustCompile(t, &p)

	// No action_risk on the input; rule must still resolve and yield valid risk.
	res := evaluate(t, module, map[string]any{"tool_id": "safe_tool", "action_type": "DATA.READ"})
	if res.Decision != opa.DecisionAllow || res.Rule.ID != "trust_tool" {
		t.Errorf("got decision=%q rule=%q, want ALLOW/trust_tool", res.Decision, res.Rule.ID)
	}
	if res.Risk != "MEDIUM" {
		t.Errorf("risk = %q, want MEDIUM fallback", res.Risk)
	}
}

func TestCompileEmptyPolicyIsDefaultOnly(t *testing.T) {
	p := StructuredPolicy{DefaultEffect: EffectAllow, AppendRiskFallbacks: ptrBool(false)}
	module := mustCompile(t, &p)

	res := evaluate(t, module, map[string]any{"action_type": "DATA.READ", "action_risk": "LOW"})
	if res.Decision != opa.DecisionAllow || res.Rule.ID != RuleDefaultAllow {
		t.Errorf("got decision=%q rule=%q, want ALLOW/%s", res.Decision, res.Rule.ID, RuleDefaultAllow)
	}
}

func TestCompileIsDeterministic(t *testing.T) {
	p := samplePolicy()
	first := mustCompile(t, p)
	second := mustCompile(t, p)
	if first != second {
		t.Errorf("Compile not deterministic:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestCompileDoesNotMutateInput(t *testing.T) {
	p := samplePolicy()
	// Rules are intentionally out of priority order to detect in-place sorting.
	p.Rules[0], p.Rules[2] = p.Rules[2], p.Rules[0]
	before := make([]string, 0, len(p.Rules))
	for i := range p.Rules {
		before = append(before, p.Rules[i].ID)
	}

	_ = mustCompile(t, p)

	for i := range p.Rules {
		if p.Rules[i].ID != before[i] {
			t.Errorf("input rules reordered at %d: got %q, want %q", i, p.Rules[i].ID, before[i])
		}
	}
}

func TestCompileCatchAllRuleShortCircuits(t *testing.T) {
	// A high-priority catch-all should win over a lower-priority specific rule.
	p := StructuredPolicy{
		DefaultEffect:       EffectDeny,
		AppendRiskFallbacks: ptrBool(false),
		Rules: []Rule{
			{ID: "catch_all", Priority: 99, Effect: EffectRequireApproval, Match: Matcher{}},
			{ID: "allow_read", Priority: 10, Effect: EffectAllow, Match: Matcher{ActionTypes: []string{"DATA.READ"}}},
		},
	}
	module := mustCompile(t, &p)

	res := evaluate(t, module, map[string]any{"action_type": "DATA.READ", "action_risk": "LOW"})
	if res.Rule.ID != "catch_all" {
		t.Errorf("rule.id = %q, want catch_all", res.Rule.ID)
	}
}
