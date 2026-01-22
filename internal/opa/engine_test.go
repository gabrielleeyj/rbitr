package opa

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/stretchr/testify/require"
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

func TestParsePriority(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    any
		expected int
		ok       bool
	}{
		{name: "float64", input: float64(5), expected: 5, ok: true},
		{name: "json number int", input: json.Number("7"), expected: 7, ok: true},
		{name: "json number float", input: json.Number("7.2"), expected: 7, ok: true},
		{name: "int", input: 3, expected: 3, ok: true},
		{name: "int64", input: int64(9), expected: 9, ok: true},
		{name: "invalid", input: "nope", ok: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parsePriority(tc.input)
			if ok != tc.ok {
				t.Fatalf("expected ok=%v got %v", tc.ok, ok)
			}
			if tc.ok && got != tc.expected {
				t.Fatalf("expected %d got %d", tc.expected, got)
			}
		})
	}
}

func TestParseResultErrors(t *testing.T) {
	t.Parallel()

	base := func() map[string]any {
		return map[string]any{
			"version":     "2026-01-20",
			"decision":    "ALLOW",
			"risk":        "LOW",
			"rule":        map[string]any{"id": "rule", "priority": 1},
			"reasons":     []any{map[string]any{"code": "ALLOW", "message": "ok"}},
			"constraints": map[string]any{},
			"tags":        []any{"tag1", 2},
		}
	}

	cases := []struct {
		name      string
		value     any
		wantErr   bool
		wantTags  []string
		wantError string
	}{
		{name: "empty result", value: nil, wantErr: true, wantError: "opa_result_empty"},
		{name: "not map", value: "nope", wantErr: true, wantError: "decode_error"},
		{
			name:      "bad decision",
			value:     func() map[string]any { v := base(); v["decision"] = "MAYBE"; return v }(),
			wantErr:   true,
			wantError: "bad_enum",
		},
		{
			name:      "missing reasons",
			value:     func() map[string]any { v := base(); v["reasons"] = []any{}; return v }(),
			wantErr:   true,
			wantError: "missing_required_field",
		},
		{
			name:     "tags filtered",
			value:    base(),
			wantErr:  false,
			wantTags: []string{"tag1"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var resultSet rego.ResultSet
			if tc.name == "empty result" {
				resultSet = rego.ResultSet{}
			} else {
				resultSet = rego.ResultSet{
					{Expressions: []*rego.ExpressionValue{{Value: tc.value}}},
				}
			}
			result, err := parseResult(resultSet)
			if tc.wantErr {
				require.Error(t, err)
				var outputErr PolicyOutputError
				require.ErrorAs(t, err, &outputErr)
				require.Equal(t, tc.wantError, outputErr.Reason)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantTags, result.Tags)
		})
	}
}
