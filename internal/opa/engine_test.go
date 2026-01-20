package opa

import (
	"context"
	"testing"
)

const samplePolicy = `package rbitr.policy

import rego.v1

default decision := {
	"version": "2026-01-20",
	"decision": "DENY",
	"risk": "LOW",
	"rule": {"id": "rule_default", "priority": 100},
	"reasons": [{"code": "DEFAULT_DENY", "message": "default"}],
	"constraints": {}
}

decision := {
	"version": "2026-01-20",
	"decision": "ALLOW",
	"risk": "LOW",
	"rule": {"id": "rule_allow", "priority": 10},
	"reasons": [{"code": "ALLOW", "message": "allow"}],
	"constraints": {}
} if {
	input.action_type == "TICKET.CREATE"
}
`

func TestEvaluatePolicy(t *testing.T) {
	cases := []struct {
		name      string
		module    string
		input     map[string]any
		expectErr bool
	}{
		{
			name:   "allow",
			module: samplePolicy,
			input:  map[string]any{"action_type": "TICKET.CREATE"},
		},
		{
			name: "invalid output",
			module: `package rbitr.policy

decision := "ALLOW"
`,
			input:     map[string]any{},
			expectErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := NewEngine(tc.module)
			result, err := engine.Evaluate(testContext(), tc.input)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Decision != "ALLOW" {
				t.Fatalf("expected ALLOW, got %s", result.Decision)
			}
		})
	}
}

func testContext() context.Context {
	return context.Background()
}
