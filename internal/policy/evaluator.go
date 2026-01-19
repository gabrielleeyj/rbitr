package policy

import (
	"context"

	"github.com/gabrielleeyj/rbitr/internal/opa"
	"github.com/gabrielleeyj/rbitr/internal/store"
)

type Result struct {
	Decision      string
	RuleID        string
	Reason        string
	PolicyVersion string
}

type Evaluator struct {
	store *store.Store
}

func NewEvaluator(store *store.Store) *Evaluator {
	return &Evaluator{store: store}
}

func (e *Evaluator) Evaluate(ctx context.Context, tenantID string, input map[string]any) (Result, error) {
	policy, err := e.store.GetPolicy(ctx, tenantID)
	if err != nil {
		return Result{}, err
	}

	engine := opa.NewEngine(policy.RegoModule)
	result, err := engine.Evaluate(ctx, input)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Decision:      result.Decision,
		RuleID:        result.RuleID,
		Reason:        result.Reason,
		PolicyVersion: result.PolicyVersion,
	}, nil
}
