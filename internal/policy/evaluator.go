package policy

import (
	"context"
	"sync"
	"time"

	"github.com/open-policy-agent/opa/v1/rego"

	"github.com/gabrielleeyj/rbitr/internal/opa"
	"github.com/gabrielleeyj/rbitr/internal/store"
)

type Result struct {
	Decision      string
	RuleID        string
	Reason        string
	PolicyVersion string
}

type EvaluatorAPI interface {
	Evaluate(ctx context.Context, tenantID string, input map[string]any) (Result, error)
}

type Evaluator struct {
	store store.StoreAPI
	mu    sync.RWMutex
	cache map[string]cachedPrepared
	ttl   time.Duration
}

type cachedPrepared struct {
	prepared  rego.PreparedEvalQuery
	module    string
	expiresAt time.Time
}

func NewEvaluator(s store.StoreAPI) EvaluatorAPI {
	return &Evaluator{
		store: s,
		cache: make(map[string]cachedPrepared),
		ttl:   5 * time.Minute,
	}
}

func (e *Evaluator) Evaluate(ctx context.Context, tenantID string, input map[string]any) (Result, error) {
	policy, err := e.store.GetPolicy(ctx, tenantID)
	if err != nil {
		return Result{}, err
	}

	cacheKey := tenantID + ":" + policy.PolicyVersion
	now := time.Now()

	e.mu.RLock()
	cached, ok := e.cache[cacheKey]
	if ok && cached.module == policy.RegoModule && now.Before(cached.expiresAt) {
		e.mu.RUnlock()
		result, err := opa.EvaluatePrepared(ctx, cached.prepared, input)
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
	e.mu.RUnlock()

	prepared, err := opa.PrepareQuery(ctx, policy.RegoModule)
	if err != nil {
		return Result{}, err
	}

	e.mu.Lock()
	e.cache[cacheKey] = cachedPrepared{
		prepared:  prepared,
		module:    policy.RegoModule,
		expiresAt: now.Add(e.ttl),
	}
	e.mu.Unlock()

	result, err := opa.EvaluatePrepared(ctx, prepared, input)
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
