package opa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/open-policy-agent/opa/v1/rego"
)

type Result struct {
	Version      string
	Decision     string
	Risk         string
	Rule         Rule
	Reasons      []Reason
	Constraints  map[string]any
	Tags         []string
	MatchedRules []MatchedRule
}

var ErrInvalidPolicyOutput = errors.New("invalid policy output")

const (
	DecisionAllow           = "ALLOW"
	DecisionDeny            = "DENY"
	DecisionRequireApproval = "REQUIRE_APPROVAL"

	riskHigh = "HIGH"

	reasonMissingRequiredField = "missing_required_field"
	reasonBadEnum              = "bad_enum"
	reasonSchemaViolation      = "schema_violation"
)

type Rule struct {
	ID       string
	Priority int
}

type Reason struct {
	Code    string
	Message string
}

type MatchedRule struct {
	RuleID             string
	Priority           int
	Effect             string
	Reasons            []Reason
	ConstraintsSummary map[string]any
}

type PolicyOutputError struct {
	Reason string
	Err    error
}

func (e PolicyOutputError) Error() string {
	if e.Err == nil {
		return "invalid policy output: " + e.Reason
	}
	return fmt.Sprintf("invalid policy output: %s: %v", e.Reason, e.Err)
}

func (e PolicyOutputError) Unwrap() error {
	return e.Err
}

type EngineAPI interface {
	Evaluate(ctx context.Context, input map[string]any) (Result, error)
}

type Engine struct {
	module string
}

func NewEngine(module string) EngineAPI {
	return &Engine{module: module}
}

func (e *Engine) Evaluate(ctx context.Context, input map[string]any) (Result, error) {
	r := rego.New(
		rego.Query("data.rbitr.policy.decision"),
		rego.Module("policy.rego", e.module),
		rego.Input(input),
	)

	resultSet, err := r.Eval(ctx)
	if err != nil {
		return Result{}, err
	}
	return parseResult(resultSet)
}

func PrepareQuery(ctx context.Context, module string) (rego.PreparedEvalQuery, error) {
	r := rego.New(
		rego.Query("data.rbitr.policy.decision"),
		rego.Module("policy.rego", module),
	)
	return r.PrepareForEval(ctx)
}

func EvaluatePrepared(ctx context.Context, prepared rego.PreparedEvalQuery, input map[string]any) (Result, error) {
	resultSet, err := prepared.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return Result{}, err
	}
	return parseResult(resultSet)
}

func parseResult(resultSet rego.ResultSet) (Result, error) {
	if len(resultSet) == 0 || len(resultSet[0].Expressions) == 0 {
		return Result{}, PolicyOutputError{Reason: "opa_result_empty", Err: ErrInvalidPolicyOutput}
	}

	value, ok := resultSet[0].Expressions[0].Value.(map[string]any)
	if !ok {
		return Result{}, PolicyOutputError{Reason: "decode_error", Err: ErrInvalidPolicyOutput}
	}

	version, ok := value["version"].(string)
	if !ok || version == "" {
		return Result{}, PolicyOutputError{Reason: reasonMissingRequiredField, Err: ErrInvalidPolicyOutput}
	}
	decision, ok := value["decision"].(string)
	if !ok || decision == "" {
		return Result{}, PolicyOutputError{Reason: reasonMissingRequiredField, Err: ErrInvalidPolicyOutput}
	}
	if !isDecisionAllowed(decision) {
		return Result{}, PolicyOutputError{Reason: reasonBadEnum, Err: ErrInvalidPolicyOutput}
	}
	risk, ok := value["risk"].(string)
	if !ok || risk == "" {
		return Result{}, PolicyOutputError{Reason: reasonMissingRequiredField, Err: ErrInvalidPolicyOutput}
	}
	if !isRiskAllowed(risk) {
		return Result{}, PolicyOutputError{Reason: reasonBadEnum, Err: ErrInvalidPolicyOutput}
	}
	ruleValue, ok := value["rule"].(map[string]any)
	if !ok {
		return Result{}, PolicyOutputError{Reason: reasonSchemaViolation, Err: ErrInvalidPolicyOutput}
	}
	ruleID, ok := ruleValue["id"].(string)
	if !ok || ruleID == "" {
		return Result{}, PolicyOutputError{Reason: reasonMissingRequiredField, Err: ErrInvalidPolicyOutput}
	}
	rulePriority, ok := parsePriority(ruleValue["priority"])
	if !ok {
		return Result{}, PolicyOutputError{Reason: reasonSchemaViolation, Err: ErrInvalidPolicyOutput}
	}
	reasons, ok := parseReasons(value["reasons"], true)
	if !ok {
		return Result{}, PolicyOutputError{Reason: reasonMissingRequiredField, Err: ErrInvalidPolicyOutput}
	}
	constraintsValue, ok := value["constraints"].(map[string]any)
	if !ok {
		return Result{}, PolicyOutputError{Reason: reasonSchemaViolation, Err: ErrInvalidPolicyOutput}
	}
	tags := []string{}
	if tagsValue, tagsOK := value["tags"].([]any); tagsOK {
		for _, tag := range tagsValue {
			if tagStr, tagOK := tag.(string); tagOK {
				tags = append(tags, tagStr)
			}
		}
	}

	matchedRules, ok := parseMatchedRules(value["matched_rules"])
	if !ok {
		return Result{}, PolicyOutputError{Reason: reasonSchemaViolation, Err: ErrInvalidPolicyOutput}
	}
	decision, resolvedRule, resolvedReasons := resolveDecision(decision, Rule{ID: ruleID, Priority: rulePriority}, reasons, matchedRules)

	return Result{
		Version:      version,
		Decision:     decision,
		Risk:         risk,
		Rule:         resolvedRule,
		Reasons:      resolvedReasons,
		Constraints:  constraintsValue,
		Tags:         tags,
		MatchedRules: matchedRules,
	}, nil
}

func parseReasons(value any, required bool) ([]Reason, bool) {
	if value == nil {
		return nil, !required
	}
	reasonsValue, ok := value.([]any)
	if !ok {
		return nil, false
	}
	if required && len(reasonsValue) == 0 {
		return nil, false
	}
	reasons := make([]Reason, 0, len(reasonsValue))
	for _, item := range reasonsValue {
		itemMap, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		code, ok := itemMap["code"].(string)
		if !ok || code == "" {
			return nil, false
		}
		message, ok := itemMap["message"].(string)
		if !ok || message == "" {
			return nil, false
		}
		reasons = append(reasons, Reason{Code: code, Message: message})
	}
	return reasons, true
}

func parseMatchedRules(value any) ([]MatchedRule, bool) {
	if value == nil {
		return nil, true
	}
	rulesValue, ok := value.([]any)
	if !ok {
		return nil, false
	}
	rules := make([]MatchedRule, 0, len(rulesValue))
	for _, item := range rulesValue {
		ruleMap, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		ruleID, ok := ruleMap["rule_id"].(string)
		if !ok || ruleID == "" {
			return nil, false
		}
		priority, ok := parsePriority(ruleMap["priority"])
		if !ok {
			return nil, false
		}
		effect, ok := parseMatchedRuleEffect(ruleMap)
		if !ok {
			return nil, false
		}
		reasons, ok := parseReasons(ruleMap["reasons"], false)
		if !ok {
			return nil, false
		}
		constraintsSummary := map[string]any{}
		if rawSummary, exists := ruleMap["constraints_summary"]; exists {
			summary, ok := rawSummary.(map[string]any)
			if !ok {
				return nil, false
			}
			constraintsSummary = summary
		}
		rules = append(rules, MatchedRule{
			RuleID:             ruleID,
			Priority:           priority,
			Effect:             effect,
			Reasons:            reasons,
			ConstraintsSummary: constraintsSummary,
		})
	}

	sort.SliceStable(rules, func(i, j int) bool {
		return matchedRuleLess(rules[i], rules[j])
	})
	return rules, true
}

func parseMatchedRuleEffect(rule map[string]any) (string, bool) {
	if effect, ok := rule["effect"].(string); ok && isDecisionAllowed(effect) {
		return effect, true
	}
	if decision, ok := rule["decision"].(string); ok && isDecisionAllowed(decision) {
		return decision, true
	}
	return "", false
}

func resolveDecision(decision string, rule Rule, reasons []Reason, matchedRules []MatchedRule) (string, Rule, []Reason) {
	if len(matchedRules) == 0 {
		return decision, rule, reasons
	}
	winner := matchedRules[0]
	resolvedReasons := reasons
	if len(winner.Reasons) > 0 {
		resolvedReasons = winner.Reasons
	}
	return winner.Effect, Rule{ID: winner.RuleID, Priority: winner.Priority}, resolvedReasons
}

func matchedRuleLess(left, right MatchedRule) bool {
	if left.Priority != right.Priority {
		return left.Priority > right.Priority
	}
	leftRank := decisionRank(left.Effect)
	rightRank := decisionRank(right.Effect)
	if leftRank != rightRank {
		return leftRank > rightRank
	}
	if left.RuleID != right.RuleID {
		return left.RuleID < right.RuleID
	}
	return false
}

func decisionRank(decision string) int {
	// Fixed governance tie-break precedence for equal-priority rules:
	// DENY > REQUIRE_APPROVAL > ALLOW.
	//nolint:mnd // this justs returns a priority.
	switch decision {
	case DecisionDeny:
		return 3
	case DecisionRequireApproval:
		return 2
	case DecisionAllow:
		return 1
	default:
		return 0
	}
}

func parsePriority(value any) (int, bool) {
	switch v := value.(type) {
	case float64:
		return int(v), true
	case json.Number:
		if parsed, err := v.Int64(); err == nil {
			return int(parsed), true
		}
		if parsed, err := v.Float64(); err == nil {
			return int(parsed), true
		}
	case int:
		return v, true
	case int64:
		return int(v), true
	}
	return 0, false
}

// IsValidDecision reports whether the given string is a valid policy decision enum.
func IsValidDecision(decision string) bool {
	return isDecisionAllowed(decision)
}

func isDecisionAllowed(decision string) bool {
	switch decision {
	case DecisionAllow, DecisionDeny, DecisionRequireApproval:
		return true
	default:
		return false
	}
}

func isRiskAllowed(risk string) bool {
	switch risk {
	case "LOW", "MEDIUM", riskHigh, "CRITICAL":
		return true
	default:
		return false
	}
}
