package policy

import (
	"context"
	"errors"
	"maps"
	"sync"
	"time"

	"github.com/open-policy-agent/opa/v1/rego"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/opa"
	"github.com/gabrielleeyj/rbitr/internal/store"
)

type Result struct {
	Version       string
	Decision      string
	Risk          string
	Rule          models.DecisionRule
	Reasons       []models.DecisionReason
	Constraints   map[string]any
	Tags          []string
	MatchedRules  []models.DecisionMatchedRule
	PolicyVersion string
}

type InvalidPolicyOutputError struct {
	Reason        string
	PolicyVersion string
	Err           error
}

func (e InvalidPolicyOutputError) Error() string {
	if e.Err == nil {
		return "invalid policy output"
	}
	return e.Err.Error()
}

func (e InvalidPolicyOutputError) Unwrap() error {
	return e.Err
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
	const defaultCacheTTL = 5 * time.Minute

	return &Evaluator{
		store: s,
		cache: make(map[string]cachedPrepared),
		ttl:   defaultCacheTTL,
	}
}

func (e *Evaluator) Evaluate(ctx context.Context, tenantID string, input map[string]any) (Result, error) {
	// Phase 1: Evaluate base policy first.
	// Base policy DENY → short-circuit, tenant policy never runs.
	// Base policy REQUIRE_APPROVAL → tenant cannot downgrade to ALLOW.
	baseResult, baseErr := EvaluateBasePolicy(ctx, input)
	if baseErr != nil {
		// Base policy failure is non-fatal; log and continue with tenant policy.
		baseResult = BasePolicyResult{Effect: "ALLOW", RuleID: "base_error", Reason: "base policy eval error"}
	}
	if baseResult.Effect == "DENY" {
		return Result{
			Version:     "base",
			Decision:    "DENY",
			Risk:        inputRisk(input),
			Rule:        models.DecisionRule{ID: baseResult.RuleID, Priority: basePolicyPriority},
			Reasons:     []models.DecisionReason{{Code: "BASE_POLICY_DENY", Message: baseResult.Reason}},
			Constraints: map[string]any{},
			Tags:        []string{"base_policy:DENY"},
		}, nil
	}

	// Phase 2: Evaluate tenant policy.
	tenantResult, err := e.evaluateTenantPolicy(ctx, tenantID, input)
	if err != nil {
		return Result{}, err
	}

	// Phase 3: Merge base policy constraint with tenant decision.
	return MergeBasePolicyDecision(baseResult, &tenantResult), nil
}

func (e *Evaluator) evaluateTenantPolicy(ctx context.Context, tenantID string, input map[string]any) (Result, error) {
	policy, err := e.store.GetPolicy(ctx, tenantID)
	if err != nil {
		return Result{}, err
	}

	inputWithVersion := make(map[string]any, len(input)+1)
	maps.Copy(inputWithVersion, input)
	inputWithVersion["policy_version"] = policy.PolicyVersion

	cacheKey := tenantID + ":" + policy.PolicyVersion
	now := time.Now()

	e.mu.RLock()
	cached, ok := e.cache[cacheKey]
	if ok && cached.module == policy.RegoModule && now.Before(cached.expiresAt) {
		e.mu.RUnlock()
		result, evalErr := opa.EvaluatePrepared(ctx, cached.prepared, inputWithVersion)
		if evalErr != nil {
			return Result{}, wrapPolicyOutputError(evalErr, policy.PolicyVersion)
		}
		return Result{
			Version:       result.Version,
			Decision:      result.Decision,
			Risk:          result.Risk,
			Rule:          models.DecisionRule{ID: result.Rule.ID, Priority: result.Rule.Priority},
			Reasons:       toDecisionReasons(result.Reasons),
			Constraints:   result.Constraints,
			Tags:          result.Tags,
			MatchedRules:  toDecisionMatchedRules(result.MatchedRules),
			PolicyVersion: policy.PolicyVersion,
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

	result, err := opa.EvaluatePrepared(ctx, prepared, inputWithVersion)
	if err != nil {
		return Result{}, wrapPolicyOutputError(err, policy.PolicyVersion)
	}

	return Result{
		Version:       result.Version,
		Decision:      result.Decision,
		Risk:          result.Risk,
		Rule:          models.DecisionRule{ID: result.Rule.ID, Priority: result.Rule.Priority},
		Reasons:       toDecisionReasons(result.Reasons),
		Constraints:   result.Constraints,
		Tags:          result.Tags,
		MatchedRules:  toDecisionMatchedRules(result.MatchedRules),
		PolicyVersion: policy.PolicyVersion,
	}, nil
}

// basePolicyPriority is the priority assigned to base policy rules.
// It is higher than any tenant policy rule to ensure base policy wins ties.
const basePolicyPriority = 1000

func inputRisk(input map[string]any) string {
	if risk, ok := input["action_risk"].(string); ok {
		return risk
	}
	return "MEDIUM"
}

func toDecisionReasons(reasons []opa.Reason) []models.DecisionReason {
	if len(reasons) == 0 {
		return nil
	}
	out := make([]models.DecisionReason, 0, len(reasons))
	for _, reason := range reasons {
		out = append(out, models.DecisionReason{Code: reason.Code, Message: reason.Message})
	}
	return out
}

func toDecisionMatchedRules(rules []opa.MatchedRule) []models.DecisionMatchedRule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]models.DecisionMatchedRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, models.DecisionMatchedRule{
			RuleID:             rule.RuleID,
			Priority:           rule.Priority,
			Effect:             rule.Effect,
			Reasons:            toDecisionReasons(rule.Reasons),
			ConstraintsSummary: rule.ConstraintsSummary,
		})
	}
	return out
}

func wrapPolicyOutputError(err error, policyVersion string) error {
	var outputErr opa.PolicyOutputError
	if errors.As(err, &outputErr) {
		return InvalidPolicyOutputError{
			Reason:        outputErr.Reason,
			PolicyVersion: policyVersion,
			Err:           err,
		}
	}
	if errors.Is(err, opa.ErrInvalidPolicyOutput) {
		return InvalidPolicyOutputError{
			Reason:        "schema_violation",
			PolicyVersion: policyVersion,
			Err:           err,
		}
	}
	return err
}
