package opa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/open-policy-agent/opa/v1/rego"
)

type Result struct {
	Version     string
	Decision    string
	Risk        string
	Rule        Rule
	Reasons     []Reason
	Constraints map[string]any
	Tags        []string
}

var ErrInvalidPolicyOutput = errors.New("invalid policy output")

type Rule struct {
	ID       string
	Priority int
}

type Reason struct {
	Code    string
	Message string
}

type PolicyOutputError struct {
	Reason string
	Err    error
}

func (e PolicyOutputError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("invalid policy output: %s", e.Reason)
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
		return Result{}, PolicyOutputError{Reason: "missing_required_field", Err: ErrInvalidPolicyOutput}
	}
	decision, ok := value["decision"].(string)
	if !ok || decision == "" {
		return Result{}, PolicyOutputError{Reason: "missing_required_field", Err: ErrInvalidPolicyOutput}
	}
	if !isDecisionAllowed(decision) {
		return Result{}, PolicyOutputError{Reason: "bad_enum", Err: ErrInvalidPolicyOutput}
	}
	risk, ok := value["risk"].(string)
	if !ok || risk == "" {
		return Result{}, PolicyOutputError{Reason: "missing_required_field", Err: ErrInvalidPolicyOutput}
	}
	if !isRiskAllowed(risk) {
		return Result{}, PolicyOutputError{Reason: "bad_enum", Err: ErrInvalidPolicyOutput}
	}
	ruleValue, ok := value["rule"].(map[string]any)
	if !ok {
		return Result{}, PolicyOutputError{Reason: "schema_violation", Err: ErrInvalidPolicyOutput}
	}
	ruleID, ok := ruleValue["id"].(string)
	if !ok || ruleID == "" {
		return Result{}, PolicyOutputError{Reason: "missing_required_field", Err: ErrInvalidPolicyOutput}
	}
	rulePriority, ok := parsePriority(ruleValue["priority"])
	if !ok {
		return Result{}, PolicyOutputError{Reason: "schema_violation", Err: ErrInvalidPolicyOutput}
	}
	reasonsValue, ok := value["reasons"].([]any)
	if !ok || len(reasonsValue) == 0 {
		return Result{}, PolicyOutputError{Reason: "missing_required_field", Err: ErrInvalidPolicyOutput}
	}
	reasons := make([]Reason, 0, len(reasonsValue))
	for _, item := range reasonsValue {
		itemMap, ok := item.(map[string]any)
		if !ok {
			return Result{}, PolicyOutputError{Reason: "schema_violation", Err: ErrInvalidPolicyOutput}
		}
		code, ok := itemMap["code"].(string)
		if !ok || code == "" {
			return Result{}, PolicyOutputError{Reason: "missing_required_field", Err: ErrInvalidPolicyOutput}
		}
		message, ok := itemMap["message"].(string)
		if !ok || message == "" {
			return Result{}, PolicyOutputError{Reason: "missing_required_field", Err: ErrInvalidPolicyOutput}
		}
		reasons = append(reasons, Reason{Code: code, Message: message})
	}
	constraintsValue, ok := value["constraints"].(map[string]any)
	if !ok {
		return Result{}, PolicyOutputError{Reason: "schema_violation", Err: ErrInvalidPolicyOutput}
	}
	tags := []string{}
	if tagsValue, ok := value["tags"].([]any); ok {
		for _, tag := range tagsValue {
			if tagStr, ok := tag.(string); ok {
				tags = append(tags, tagStr)
			}
		}
	}

	return Result{
		Version:     version,
		Decision:    decision,
		Risk:        risk,
		Rule:        Rule{ID: ruleID, Priority: rulePriority},
		Reasons:     reasons,
		Constraints: constraintsValue,
		Tags:        tags,
	}, nil
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

func isDecisionAllowed(decision string) bool {
	switch decision {
	case "ALLOW", "DENY", "REQUIRE_APPROVAL":
		return true
	default:
		return false
	}
}

func isRiskAllowed(risk string) bool {
	switch risk {
	case "LOW", "MEDIUM", "HIGH", "CRITICAL":
		return true
	default:
		return false
	}
}
