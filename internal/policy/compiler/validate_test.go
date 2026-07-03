package compiler

import (
	"errors"
	"testing"
)

func baseValidPolicy() *StructuredPolicy {
	return &StructuredPolicy{
		SchemaVersion: "1",
		DefaultEffect: EffectDeny,
		Rules: []Rule{
			{ID: "allow_reads", Priority: 100, Effect: EffectAllow, Match: Matcher{ActionTypes: []string{"DATA.READ"}}},
		},
	}
}

func TestValidateAcceptsValidPolicy(t *testing.T) {
	if err := Validate(baseValidPolicy()); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidateRejects(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*StructuredPolicy)
		wantField string
	}{
		{
			name:      "invalid default effect",
			mutate:    func(p *StructuredPolicy) { p.DefaultEffect = "MAYBE" },
			wantField: "default_effect",
		},
		{
			name:      "invalid rule effect",
			mutate:    func(p *StructuredPolicy) { p.Rules[0].Effect = "PERHAPS" },
			wantField: "rules[0].effect",
		},
		{
			name:      "unknown action type",
			mutate:    func(p *StructuredPolicy) { p.Rules[0].Match.ActionTypes = []string{"NOPE.WHATEVER"} },
			wantField: "rules[0].match.action_types",
		},
		{
			name:      "unknown risk",
			mutate:    func(p *StructuredPolicy) { p.Rules[0].Match.ActionRisks = []string{"SPICY"} },
			wantField: "rules[0].match.action_risks",
		},
		{
			name:      "bad rule id",
			mutate:    func(p *StructuredPolicy) { p.Rules[0].ID = "Bad-ID!" },
			wantField: "rules[0].id",
		},
		{
			name:      "reserved rule id",
			mutate:    func(p *StructuredPolicy) { p.Rules[0].ID = RuleDefaultDeny },
			wantField: "rules[0].id",
		},
		{
			name: "duplicate rule id",
			mutate: func(p *StructuredPolicy) {
				p.Rules = append(p.Rules, Rule{ID: "allow_reads", Priority: 5, Effect: EffectDeny})
			},
			wantField: "rules[1].id",
		},
		{
			name:      "priority out of range",
			mutate:    func(p *StructuredPolicy) { p.Rules[0].Priority = 5000 },
			wantField: "rules[0].priority",
		},
		{
			name: "approval on non-approval effect",
			mutate: func(p *StructuredPolicy) {
				p.Rules[0].Approval = &ApprovalConstraint{ExpiresInSeconds: 900}
			},
			wantField: "rules[0].approval",
		},
		{
			name: "approval ttl out of range",
			mutate: func(p *StructuredPolicy) {
				p.Rules[0].Effect = EffectRequireApproval
				p.Rules[0].Approval = &ApprovalConstraint{ExpiresInSeconds: 5}
			},
			wantField: "rules[0].approval.expires_in_seconds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := baseValidPolicy()
			tt.mutate(p)

			err := Validate(p)
			if err == nil {
				t.Fatalf("Validate() = nil, want error for %s", tt.name)
			}
			if !errors.Is(err, ErrInvalidPolicy) {
				t.Errorf("error does not wrap ErrInvalidPolicy: %v", err)
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("error is not *ValidationError: %v", err)
			}
			if _, ok := ve.Fields[tt.wantField]; !ok {
				t.Errorf("missing field %q in %v", tt.wantField, ve.Fields)
			}
		})
	}
}

func TestValidateAllowsActionPrefix(t *testing.T) {
	p := baseValidPolicy()
	p.Rules[0].Match.ActionTypes = []string{"MCP.*"}
	if err := Validate(p); err != nil {
		t.Errorf("Validate() error = %v, want nil for prefix form", err)
	}
}

func TestCompileReturnsValidationError(t *testing.T) {
	p := baseValidPolicy()
	p.DefaultEffect = "BOGUS"
	if _, err := Compile(p); !errors.Is(err, ErrInvalidPolicy) {
		t.Errorf("Compile() error = %v, want ErrInvalidPolicy", err)
	}
}
