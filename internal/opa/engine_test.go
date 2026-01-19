package opa

import (
	"context"
	"testing"
)

const samplePolicy = `package rbitr.policy

default decision = {"decision": "DENY", "rule_id": "rule_default", "reason": "default", "policy_version": "p_v1"}

decision := {"decision": "ALLOW", "rule_id": "rule_allow", "reason": "allow", "policy_version": "p_v1"} {
	input.action_type == "TICKET.CREATE"
}
`

func TestEvaluatePolicy(t *testing.T) {
	engine := NewEngine(samplePolicy)
	result, err := engine.Evaluate(testContext(), map[string]interface{}{"action_type": "TICKET.CREATE"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Decision != "ALLOW" {
		t.Fatalf("expected ALLOW, got %s", result.Decision)
	}
}

func testContext() context.Context {
	return context.Background()
}
