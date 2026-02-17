package public

import (
	"maps"

	"github.com/gabrielleeyj/rbitr/internal/models"
)

func withMatchedRulesConstraint(constraints map[string]any, matchedRules []models.DecisionMatchedRule) map[string]any {
	cloned := make(map[string]any, len(constraints)+1)
	maps.Copy(cloned, constraints)
	if len(matchedRules) > 0 {
		cloned["matched_rules"] = decisionMatchedRulesAsMaps(matchedRules)
	}
	return cloned
}

func decisionMatchedRulesAsMaps(rules []models.DecisionMatchedRule) []map[string]any {
	out := make([]map[string]any, 0, len(rules))
	for _, rule := range rules {
		item := map[string]any{
			"rule_id":  rule.RuleID,
			"priority": rule.Priority,
			"effect":   rule.Effect,
		}
		if len(rule.Reasons) > 0 {
			item["reasons"] = decisionReasonsAsMaps(rule.Reasons)
		}
		if len(rule.ConstraintsSummary) > 0 {
			item["constraints_summary"] = rule.ConstraintsSummary
		}
		out = append(out, item)
	}
	return out
}

func decisionReasonsAsMaps(reasons []models.DecisionReason) []map[string]any {
	out := make([]map[string]any, 0, len(reasons))
	for _, reason := range reasons {
		item := map[string]any{
			"code":    reason.Code,
			"message": reason.Message,
		}
		out = append(out, item)
	}
	return out
}

func matchedRulesFromConstraints(constraints map[string]any) []map[string]any {
	raw, ok := constraints["matched_rules"]
	if !ok {
		return nil
	}
	values, ok := raw.([]map[string]any)
	if ok {
		return values
	}
	anySlice, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(anySlice))
	for _, item := range anySlice {
		value, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
