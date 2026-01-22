package policy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/opa"
	"github.com/gabrielleeyj/rbitr/internal/store"
)

const allowPolicy = `package rbitr.policy

import rego.v1

default decision := {
	"version": "2026-01-20",
	"decision": "DENY",
	"risk": "LOW",
	"rule": {"id": "rule_default", "priority": 100},
	"reasons": [{"code": "DEFAULT_DENY", "message": "default"}],
	"constraints": {}
}

decision := {
	"version": "2026-01-20",
	"decision": "ALLOW",
	"risk": "LOW",
	"rule": {"id": "rule_allow", "priority": 10},
	"reasons": [{"code": "ALLOW", "message": "ok"}],
	"constraints": {}
} if {
	input.action_type == "TICKET.CREATE"
}
`

const invalidPolicy = `package rbitr.policy

decision := "ALLOW"
`

func TestEvaluatorEvaluate(t *testing.T) {
	cases := []struct {
		name     string
		policy   models.Policy
		storeErr error
		input    map[string]any
		expected Result
		wantErr  bool
	}{
		{
			name:     "store error",
			storeErr: errors.New("store down"),
			wantErr:  true,
		},
		{
			name:   "allow policy",
			policy: models.Policy{PolicyID: "p1", TenantID: "t1", RegoModule: allowPolicy, PolicyVersion: "p_v1"},
			input:  map[string]any{"action_type": "TICKET.CREATE"},
			expected: Result{
				Version:       "2026-01-20",
				Decision:      "ALLOW",
				Risk:          "LOW",
				Rule:          models.DecisionRule{ID: "rule_allow", Priority: 10},
				Reasons:       []models.DecisionReason{{Code: "ALLOW", Message: "ok"}},
				Constraints:   map[string]any{},
				Tags:          []string{},
				PolicyVersion: "p_v1",
			},
		},
		{
			name:    "invalid policy output",
			policy:  models.Policy{PolicyID: "p2", TenantID: "t1", RegoModule: invalidPolicy, PolicyVersion: "p_v1"},
			input:   map[string]any{},
			wantErr: true,
		},
		{
			name:    "invalid rego",
			policy:  models.Policy{PolicyID: "p3", TenantID: "t1", RegoModule: "package rbitr.policy\n\nbad", PolicyVersion: "p_v1"},
			input:   map[string]any{},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storeMock := store.NewMockStoreAPI(t)
			if tc.storeErr != nil {
				storeMock.On("GetPolicy", context.Background(), "t1").Return(models.Policy{}, tc.storeErr)
			} else {
				storeMock.On("GetPolicy", context.Background(), "t1").Return(tc.policy, nil)
			}

			var storeAPI store.StoreAPI = storeMock
			evaluator := NewEvaluator(storeAPI)
			result, err := evaluator.Evaluate(context.Background(), "t1", tc.input)
			if tc.wantErr {
				require.Error(t, err)
				if tc.policy.RegoModule == invalidPolicy {
					require.ErrorIs(t, err, opa.ErrInvalidPolicyOutput)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestEvaluatorCacheHit(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetPolicy", context.Background(), "t1").
		Return(models.Policy{PolicyID: "p1", TenantID: "t1", RegoModule: invalidPolicy, PolicyVersion: "p_v1"}, nil)

	evalIface := NewEvaluator(storeMock)
	evaluator := evalIface.(*Evaluator)

	prepared, err := opa.PrepareQuery(context.Background(), allowPolicy)
	require.NoError(t, err)

	evaluator.cache["t1:p_v1"] = cachedPrepared{
		prepared:  prepared,
		module:    invalidPolicy,
		expiresAt: time.Now().Add(time.Minute),
	}

	result, err := evaluator.Evaluate(context.Background(), "t1", map[string]any{"action_type": "TICKET.CREATE"})
	require.NoError(t, err)
	require.Equal(t, "ALLOW", result.Decision)
	require.Equal(t, "LOW", result.Risk)
	require.Equal(t, "rule_allow", result.Rule.ID)
}

func TestEvaluatorCacheExpired(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetPolicy", context.Background(), "t1").
		Return(models.Policy{PolicyID: "p1", TenantID: "t1", RegoModule: allowPolicy, PolicyVersion: "p_v1"}, nil)

	evalIface := NewEvaluator(storeMock)
	evaluator := evalIface.(*Evaluator)

	prepared, err := opa.PrepareQuery(context.Background(), allowPolicy)
	require.NoError(t, err)

	evaluator.cache["t1:p_v1"] = cachedPrepared{
		prepared:  prepared,
		module:    allowPolicy,
		expiresAt: time.Now().Add(-time.Minute),
	}

	result, err := evaluator.Evaluate(context.Background(), "t1", map[string]any{"action_type": "TICKET.CREATE"})
	require.NoError(t, err)
	require.Equal(t, "ALLOW", result.Decision)
}

func TestToDecisionReasons(t *testing.T) {
	t.Parallel()

	if got := toDecisionReasons(nil); got != nil {
		t.Fatalf("expected nil reasons")
	}

	reasons := toDecisionReasons([]opa.Reason{{Code: "C", Message: "M"}})
	if len(reasons) != 1 || reasons[0].Code != "C" {
		t.Fatalf("unexpected reasons: %+v", reasons)
	}
}

func TestWrapPolicyOutputError(t *testing.T) {
	t.Parallel()

	err := wrapPolicyOutputError(opa.PolicyOutputError{Reason: "bad_enum", Err: opa.ErrInvalidPolicyOutput}, "p_v1")
	var invalidErr InvalidPolicyOutputError
	require.ErrorAs(t, err, &invalidErr)
	require.Equal(t, "bad_enum", invalidErr.Reason)
	require.Equal(t, "p_v1", invalidErr.PolicyVersion)

	err = wrapPolicyOutputError(opa.ErrInvalidPolicyOutput, "p_v2")
	require.ErrorAs(t, err, &invalidErr)
	require.Equal(t, "schema_violation", invalidErr.Reason)
	require.Equal(t, "p_v2", invalidErr.PolicyVersion)
}
