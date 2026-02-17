package public

import "testing"

func TestParsePolicyRateLimitOverride(t *testing.T) {
	overrides := parsePolicyRateLimitOverride(map[string]any{
		"rate_limit": map[string]any{
			"per_minute": float64(12),
			"per_day":    float64(345),
			"scope":      "tenant_tool",
		},
	})

	if overrides.PerMinute == nil || *overrides.PerMinute != 12 {
		t.Fatalf("expected per_minute override")
	}
	if overrides.PerDay == nil || *overrides.PerDay != 345 {
		t.Fatalf("expected per_day override")
	}
	if overrides.Scope == nil || *overrides.Scope != "tenant_tool" {
		t.Fatalf("expected scope override")
	}
}

func TestApplyRateLimitScope(t *testing.T) {
	key := rateLimitKey{
		tenantID:   "t1",
		agentID:    "agent",
		toolID:     "tool",
		actionType: "DATA.EXPORT",
	}

	tests := []struct {
		name           string
		scope          string
		expectedAgent  string
		expectedTool   string
		expectedAction string
	}{
		{
			name:           "tenant",
			scope:          rateLimitScopeTenant,
			expectedAgent:  "",
			expectedTool:   "",
			expectedAction: "",
		},
		{
			name:           "tenant_agent",
			scope:          rateLimitScopeTenantAgent,
			expectedAgent:  "agent",
			expectedTool:   "",
			expectedAction: "",
		},
		{
			name:           "tenant_tool",
			scope:          rateLimitScopeTenantTool,
			expectedAgent:  "",
			expectedTool:   "tool",
			expectedAction: "",
		},
		{
			name:           "tenant_agent_tool",
			scope:          rateLimitScopeTenantAgentTool,
			expectedAgent:  "agent",
			expectedTool:   "tool",
			expectedAction: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scoped := applyRateLimitScope(tc.scope, key)
			if scoped.agentID != tc.expectedAgent {
				t.Fatalf("expected agent %q got %q", tc.expectedAgent, scoped.agentID)
			}
			if scoped.toolID != tc.expectedTool {
				t.Fatalf("expected tool %q got %q", tc.expectedTool, scoped.toolID)
			}
			if scoped.actionType != tc.expectedAction {
				t.Fatalf("expected action %q got %q", tc.expectedAction, scoped.actionType)
			}
		})
	}
}
