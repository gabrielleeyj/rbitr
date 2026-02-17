package public

import (
	"testing"

	"github.com/gabrielleeyj/rbitr/internal/config"
)

func TestEnforceArgumentConstraints_DenyRule(t *testing.T) {
	deps := &Dependencies{
		Config: config.Config{FeatureArgConstraints: true},
	}

	constraints := map[string]any{
		"args": map[string]any{
			"deny": []any{
				map[string]any{
					"id":      "deny_prod_repo",
					"path":    "/repo/name",
					"op":      "prefix",
					"value":   "prod-",
					"message": "production repos are blocked",
				},
			},
		},
	}
	arguments := map[string]any{
		"repo": map[string]any{
			"name": "prod-payments",
		},
	}

	violation := deps.enforceArgumentConstraints(constraints, arguments)
	if violation == nil {
		t.Fatalf("expected violation")
	}
	if violation.ReasonCode != argConstraintReasonDeny {
		t.Fatalf("expected reason %q got %q", argConstraintReasonDeny, violation.ReasonCode)
	}
	if violation.Message != "production repos are blocked" {
		t.Fatalf("expected custom message")
	}
	if len(violation.Failures) != 1 {
		t.Fatalf("expected one failure")
	}
	if violation.Failures[0].Path != "/repo/name" {
		t.Fatalf("unexpected path")
	}
}

func TestEnforceArgumentConstraints_AllowRulesNotAllowed(t *testing.T) {
	deps := &Dependencies{
		Config: config.Config{FeatureArgConstraints: true},
	}

	constraints := map[string]any{
		"args": map[string]any{
			"allow": []any{
				map[string]any{"path": "/branch", "op": "eq", "value": "main"},
				map[string]any{"path": "/branch", "op": "eq", "value": "release"},
			},
		},
	}
	arguments := map[string]any{"branch": "feature/foo"}

	violation := deps.enforceArgumentConstraints(constraints, arguments)
	if violation == nil {
		t.Fatalf("expected violation")
	}
	if violation.ReasonCode != argConstraintReasonNotAllowed {
		t.Fatalf("expected reason %q got %q", argConstraintReasonNotAllowed, violation.ReasonCode)
	}
	if len(violation.Failures) != 1 {
		t.Fatalf("expected one failure")
	}
	if violation.Failures[0].Op != "eq" {
		t.Fatalf("expected op eq")
	}
}

func TestEnforceArgumentConstraints_AllowRulesMatch(t *testing.T) {
	deps := &Dependencies{
		Config: config.Config{FeatureArgConstraints: true},
	}

	constraints := map[string]any{
		"args": map[string]any{
			"allow": []any{
				map[string]any{"path": "/repo/name", "op": "regex", "value": "^svc-[a-z0-9-]+$"},
				map[string]any{"path": "/branch", "op": "eq", "value": "main"},
			},
		},
	}
	arguments := map[string]any{
		"repo":   map[string]any{"name": "svc-billing"},
		"branch": "main",
	}

	violation := deps.enforceArgumentConstraints(constraints, arguments)
	if violation != nil {
		t.Fatalf("expected no violation, got %+v", violation)
	}
}

func TestEnforceArgumentConstraints_JSONSchema(t *testing.T) {
	deps := &Dependencies{
		Config: config.Config{FeatureArgConstraints: true},
	}

	constraints := map[string]any{
		"args": map[string]any{
			"allow": []any{
				map[string]any{
					"path": "/payload",
					"op":   "jsonschema",
					"value": map[string]any{
						"type":       "object",
						"required":   []any{"amount"},
						"properties": map[string]any{"amount": map[string]any{"type": "number"}},
					},
				},
			},
		},
	}
	arguments := map[string]any{
		"payload": map[string]any{
			"amount": float64(10),
		},
	}

	violation := deps.enforceArgumentConstraints(constraints, arguments)
	if violation != nil {
		t.Fatalf("expected no violation")
	}
}

func TestParseRESTArguments(t *testing.T) {
	parsed := parseRESTArguments([]byte(`{"branch":"main"}`))
	argMap, ok := parsed.(map[string]any)
	if !ok {
		t.Fatalf("expected object arguments")
	}
	if argMap["branch"] != "main" {
		t.Fatalf("expected parsed branch")
	}

	raw := parseRESTArguments([]byte(`not-json`))
	if _, ok := raw.(string); !ok {
		t.Fatalf("expected string fallback")
	}
}
