package opa

import (
	"context"

	"github.com/open-policy-agent/opa/v1/rego"
)

type Result struct {
	Decision      string
	RuleID        string
	Reason        string
	PolicyVersion string
}

type Engine struct {
	module string
}

func NewEngine(module string) *Engine {
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
	if len(resultSet) == 0 || len(resultSet[0].Expressions) == 0 {
		return Result{}, nil
	}

	value, ok := resultSet[0].Expressions[0].Value.(map[string]any)
	if !ok {
		return Result{}, nil
	}

	res := Result{}
	if decision, ok := value["decision"].(string); ok {
		res.Decision = decision
	}
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
