package opa

import (
	"context"
	"errors"

	"github.com/open-policy-agent/opa/v1/rego"
)

type Result struct {
	Decision      string
	RuleID        string
	Reason        string
	PolicyVersion string
}

var ErrInvalidPolicyOutput = errors.New("invalid policy output")

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
		return Result{}, ErrInvalidPolicyOutput
	}

	value, ok := resultSet[0].Expressions[0].Value.(map[string]any)
	if !ok {
		return Result{}, ErrInvalidPolicyOutput
	}

	decision, ok := value["decision"].(string)
	if !ok || decision == "" {
		return Result{}, ErrInvalidPolicyOutput
	}

	res := Result{Decision: decision}
	if ruleID, ok := value["rule_id"].(string); ok {
		res.RuleID = ruleID
	}
	if reason, ok := value["reason"].(string); ok {
		res.Reason = reason
	}
	if policyVersion, ok := value["policy_version"].(string); ok {
		res.PolicyVersion = policyVersion
	}

	return res, nil
}
