package compiler

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/gabrielleeyj/rbitr/internal/classification"
)

// ErrInvalidPolicy is the sentinel wrapped by all validation failures.
var ErrInvalidPolicy = errors.New("invalid structured policy")

// ValidationError reports one or more field-level problems in a structured
// policy. It is safe to surface to API callers.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	if len(e.Fields) == 0 {
		return ErrInvalidPolicy.Error()
	}
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s: %s", k, e.Fields[k]))
	}
	return fmt.Sprintf("%s: %s", ErrInvalidPolicy.Error(), strings.Join(parts, ", "))
}

func (e *ValidationError) Unwrap() error { return ErrInvalidPolicy }

var (
	ruleIDPattern       = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)
	actionPrefixPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*\.\*$`)
)

func validEffects() map[Effect]struct{} {
	return map[Effect]struct{}{
		EffectAllow:           {},
		EffectDeny:            {},
		EffectRequireApproval: {},
	}
}

func validRisks() map[string]struct{} {
	return map[string]struct{}{riskLow: {}, riskMedium: {}, riskHigh: {}, riskCritical: {}}
}

func reservedRuleIDs() map[string]struct{} {
	return map[string]struct{}{
		RuleDefaultDeny:         {},
		RuleDefaultAllow:        {},
		RuleDefaultApproval:     {},
		RuleHighRiskUnknown:     {},
		RuleCriticalRiskUnknown: {},
	}
}

func actionTypeSet() map[string]struct{} {
	set := map[string]struct{}{}
	for _, at := range classification.ActionTypes() {
		set[at] = struct{}{}
	}
	return set
}

// Validate checks a structured policy at the system boundary. It returns a
// *ValidationError (wrapping ErrInvalidPolicy) describing every problem found.
func Validate(p *StructuredPolicy) error {
	fields := map[string]string{}

	effects := validEffects()
	if _, ok := effects[p.DefaultEffect]; !ok {
		fields["default_effect"] = "must be ALLOW, DENY, or REQUIRE_APPROVAL"
	}

	risks := validRisks()
	actionTypes := actionTypeSet()
	reserved := reservedRuleIDs()
	seenIDs := map[string]struct{}{}

	for i := range p.Rules {
		r := &p.Rules[i]
		prefix := fmt.Sprintf("rules[%d]", i)

		validateRuleID(fields, prefix, r.ID, reserved, seenIDs)

		if _, ok := effects[r.Effect]; !ok {
			fields[prefix+".effect"] = "must be ALLOW, DENY, or REQUIRE_APPROVAL"
		}
		if r.Priority < minRulePriority || r.Priority > maxRulePriority {
			fields[prefix+".priority"] = fmt.Sprintf("must be between %d and %d", minRulePriority, maxRulePriority)
		}

		validateActionTypes(fields, prefix, r.Match.ActionTypes, actionTypes)
		validateActionRisks(fields, prefix, r.Match.ActionRisks, risks)
		validateApproval(fields, prefix, r)
	}

	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

func validateRuleID(fields map[string]string, prefix, id string, reserved, seen map[string]struct{}) {
	switch {
	case !ruleIDPattern.MatchString(id):
		fields[prefix+".id"] = "must match ^[a-z0-9_]{1,64}$"
	case isReserved(id, reserved):
		fields[prefix+".id"] = "must not use a reserved rule id"
	default:
		if _, dup := seen[id]; dup {
			fields[prefix+".id"] = "duplicate rule id"
		}
		seen[id] = struct{}{}
	}
}

func isReserved(id string, reserved map[string]struct{}) bool {
	_, ok := reserved[id]
	return ok
}

func validateActionTypes(fields map[string]string, prefix string, values []string, known map[string]struct{}) {
	for _, at := range values {
		if _, ok := known[at]; ok {
			continue
		}
		if actionPrefixPattern.MatchString(at) {
			continue
		}
		fields[prefix+".match.action_types"] = fmt.Sprintf("unknown action type %q (use a known type or a prefix like \"MCP.*\")", at)
		return
	}
}

func validateActionRisks(fields map[string]string, prefix string, values []string, known map[string]struct{}) {
	for _, risk := range values {
		if _, ok := known[risk]; !ok {
			fields[prefix+".match.action_risks"] = fmt.Sprintf("unknown risk %q (use LOW, MEDIUM, HIGH, or CRITICAL)", risk)
			return
		}
	}
}

func validateApproval(fields map[string]string, prefix string, r *Rule) {
	if r.Approval == nil {
		return
	}
	if r.Effect != EffectRequireApproval {
		fields[prefix+".approval"] = "approval is only valid for REQUIRE_APPROVAL rules"
		return
	}
	if r.Approval.ExpiresInSeconds != 0 &&
		(r.Approval.ExpiresInSeconds < minApprovalTTLSeconds || r.Approval.ExpiresInSeconds > maxApprovalTTLSeconds) {
		fields[prefix+".approval.expires_in_seconds"] = fmt.Sprintf("must be between %d and %d", minApprovalTTLSeconds, maxApprovalTTLSeconds)
	}
}
