package compiler

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// codeForEffect returns the deterministic reason code for a rule effect.
func codeForEffect(e Effect) string {
	switch e {
	case EffectAllow:
		return "ALLOW"
	case EffectDeny:
		return "DENY"
	case EffectRequireApproval:
		return "APPROVAL_REQUIRED"
	default:
		return "UNKNOWN"
	}
}

// messageForRule returns the rule's reason text or a deterministic default.
func messageForRule(r *Rule) string {
	if strings.TrimSpace(r.Reason) != "" {
		return r.Reason
	}
	return fmt.Sprintf("Policy: %s by rule %s", r.Effect, r.ID)
}

// render emits the full Rego module for the already-sorted rule slice.
func render(p *StructuredPolicy, sorted []Rule) string {
	var b strings.Builder

	b.WriteString("package rbitr.policy\n\n")
	b.WriteString("import rego.v1\n\n")
	writeHelpers(&b, p.outputVersion())

	// Safe risk binding so rules that don't match on risk still yield a valid
	// risk value, and so a request missing action_risk never leaves rules undefined.
	fmt.Fprintf(&b, "req_risk := object.get(input, \"action_risk\", %s)\n\n", strconv.Quote(defaultFallbackRisk))

	writeActionMatchHelpers(&b, sorted)

	writeDecisionChain(&b, p, sorted)

	return b.String()
}

// writeHelpers emits the decision_obj / decision_obj_c constructors.
func writeHelpers(b *strings.Builder, outputVersion string) {
	b.WriteString("decision_obj(decision, risk, rule_id, priority, code, message) := ")
	b.WriteString("decision_obj_c(decision, risk, rule_id, priority, code, message, {})\n\n")

	b.WriteString("decision_obj_c(decision, risk, rule_id, priority, code, message, constraints) := {\n")
	fmt.Fprintf(b, "\t\"version\": %s,\n", strconv.Quote(outputVersion))
	b.WriteString("\t\"decision\": decision,\n")
	b.WriteString("\t\"risk\": risk,\n")
	b.WriteString("\t\"rule\": {\"id\": rule_id, \"priority\": priority},\n")
	b.WriteString("\t\"reasons\": [{\"code\": code, \"message\": message}],\n")
	b.WriteString("\t\"constraints\": constraints,\n")
	b.WriteString("\t\"tags\": []\n")
	b.WriteString("}\n\n")
}

// writeActionMatchHelpers emits per-rule OR helpers for action-type matching
// only where an OR is required (any prefix present, or a mix of exact + prefix,
// or multiple prefixes). Rules matched by a single inline condition emit nothing.
func writeActionMatchHelpers(b *strings.Builder, sorted []Rule) {
	for i := range sorted {
		r := &sorted[i]
		exact, prefixes := splitActionTypes(r.Match.ActionTypes)
		if !needsActionHelper(exact, prefixes) {
			continue
		}
		name := actionHelperName(r.ID)
		if len(exact) > 0 {
			fmt.Fprintf(b, "%s if input.action_type in %s\n", name, regoStringSet(exact))
		}
		for _, prefix := range prefixes {
			fmt.Fprintf(b, "%s if startswith(input.action_type, %s)\n", name, strconv.Quote(prefix))
		}
		b.WriteString("\n")
	}
}

// writeDecisionChain emits the else-chained decision rules and the terminal default.
func writeDecisionChain(b *strings.Builder, p *StructuredPolicy, sorted []Rule) {
	first := true
	for i := range sorted {
		r := &sorted[i]
		head := ruleHead(r)
		body := ruleBody(r)
		if first {
			b.WriteString("decision := " + head + " if {\n" + body + "}")
			first = false
			continue
		}
		b.WriteString(" else := " + head + " if {\n" + body + "}")
	}

	if p.appendRiskFallbacks() {
		writeRiskFallback(b, &first, RuleHighRiskUnknown, "HIGH", "HIGH_RISK_UNKNOWN")
		writeRiskFallback(b, &first, RuleCriticalRiskUnknown, "CRITICAL", "CRITICAL_RISK_UNKNOWN")
	}

	writeDefaultRule(b, p, first)
	b.WriteString("\n")
}

func writeRiskFallback(b *strings.Builder, first *bool, ruleID, risk, code string) {
	head := fmt.Sprintf("decision_obj(%q, req_risk, %q, %d, %q, %q)",
		string(EffectRequireApproval), ruleID, fallbackRulePriority, code, "Policy: approval required for high/critical risk")
	body := "\treq_risk == " + strconv.Quote(risk) + "\n"
	if *first {
		b.WriteString("decision := " + head + " if {\n" + body + "}")
		*first = false
		return
	}
	b.WriteString(" else := " + head + " if {\n" + body + "}")
}

func writeDefaultRule(b *strings.Builder, p *StructuredPolicy, first bool) {
	ruleID, code := defaultRuleMeta(p.DefaultEffect)
	message := fmt.Sprintf("Default %s: no matching rule", strings.ToLower(string(p.DefaultEffect)))
	head := fmt.Sprintf("decision_obj(%q, %s, %q, %d, %q, %q)",
		string(p.DefaultEffect), strconv.Quote(defaultFallbackRisk), ruleID, defaultRulePriority, code, message)
	if first {
		// No user rules and no risk fallbacks: emit an unconditional decision.
		b.WriteString("decision := " + head)
		return
	}
	b.WriteString(" else := " + head)
}

func defaultRuleMeta(e Effect) (ruleID, code string) {
	switch e {
	case EffectAllow:
		return RuleDefaultAllow, "DEFAULT_ALLOW"
	case EffectRequireApproval:
		return RuleDefaultApproval, "DEFAULT_REQUIRE_APPROVAL"
	default:
		return RuleDefaultDeny, "DEFAULT_DENY"
	}
}

// ruleHead builds the decision_obj / decision_obj_c constructor call for a rule.
func ruleHead(r *Rule) string {
	code := codeForEffect(r.Effect)
	message := messageForRule(r)
	if r.Effect == EffectRequireApproval && r.Approval != nil {
		constraints := approvalConstraints(r.Approval)
		return fmt.Sprintf("decision_obj_c(%q, req_risk, %q, %d, %q, %q, %s)",
			string(r.Effect), r.ID, r.Priority, code, message, constraints)
	}
	return fmt.Sprintf("decision_obj(%q, req_risk, %q, %d, %q, %q)",
		string(r.Effect), r.ID, r.Priority, code, message)
}

func approvalConstraints(a *ApprovalConstraint) string {
	ttl := a.ExpiresInSeconds
	if ttl == 0 {
		ttl = defaultApprovalTTLSeconds
	}
	reason := a.Reason
	if strings.TrimSpace(reason) == "" {
		reason = "Approval required"
	}
	return fmt.Sprintf("{\"approval\": {\"expires_in_seconds\": %d, \"reason\": %s}}", ttl, strconv.Quote(reason))
}

// ruleBody builds the indented condition lines for a rule. A catch-all matcher
// yields a body of "true" so it forms a valid (unconditional) else clause.
func ruleBody(r *Rule) string {
	conds := ruleConditions(r)
	if len(conds) == 0 {
		return "\ttrue\n"
	}
	var b strings.Builder
	for _, c := range conds {
		b.WriteString("\t" + c + "\n")
	}
	return b.String()
}

// ruleConditions returns the deterministic, AND-ed condition expressions for a
// rule's matcher, in fixed order: tool_id, action_type, action_risk.
func ruleConditions(r *Rule) []string {
	var conds []string

	if c := toolCondition(r.Match.ToolIDs); c != "" {
		conds = append(conds, c)
	}
	if c := actionTypeCondition(r); c != "" {
		conds = append(conds, c)
	}
	if c := riskCondition(r.Match.ActionRisks); c != "" {
		conds = append(conds, c)
	}
	return conds
}

func toolCondition(toolIDs []string) string {
	values := dedupeSorted(toolIDs)
	switch len(values) {
	case 0:
		return ""
	case 1:
		return "input.tool_id == " + strconv.Quote(values[0])
	default:
		return "input.tool_id in " + regoStringSet(values)
	}
}

func actionTypeCondition(r *Rule) string {
	exact, prefixes := splitActionTypes(r.Match.ActionTypes)
	if len(exact) == 0 && len(prefixes) == 0 {
		return ""
	}
	if needsActionHelper(exact, prefixes) {
		return actionHelperName(r.ID)
	}
	if len(prefixes) == 1 {
		return "startswith(input.action_type, " + strconv.Quote(prefixes[0]) + ")"
	}
	return "input.action_type in " + regoStringSet(exact)
}

func riskCondition(risks []string) string {
	values := dedupeSorted(risks)
	switch len(values) {
	case 0:
		return ""
	case 1:
		return "input.action_risk == " + strconv.Quote(values[0])
	default:
		return "input.action_risk in " + regoStringSet(values)
	}
}

// needsActionHelper reports whether OR semantics across kinds require a helper
// rule (more than one clause among the exact-set and per-prefix checks).
func needsActionHelper(exact, prefixes []string) bool {
	clauses := len(prefixes)
	if len(exact) > 0 {
		clauses++
	}
	return clauses > 1
}

func actionHelperName(ruleID string) string {
	return "action_match_" + ruleID
}

// splitActionTypes separates exact action types from prefix forms ("MCP.*" ->
// "MCP."). Both slices are deduped and sorted for determinism.
func splitActionTypes(actionTypes []string) (exact, prefixes []string) {
	exactSet := map[string]struct{}{}
	prefixSet := map[string]struct{}{}
	for _, at := range actionTypes {
		if strings.HasSuffix(at, ".*") {
			prefixSet[strings.TrimSuffix(at, "*")] = struct{}{}
			continue
		}
		exactSet[at] = struct{}{}
	}
	return sortedKeys(exactSet), sortedKeys(prefixSet)
}

func regoStringSet(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, v := range values {
		quoted = append(quoted, strconv.Quote(v))
	}
	return "{" + strings.Join(quoted, ", ") + "}"
}

func dedupeSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}
	return sortedKeys(set)
}

func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
