package policy

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/opa"
	"github.com/gabrielleeyj/rbitr/internal/store"
)

const allowPolicy = `package rbitr.policy

default decision := {"decision": "DENY", "rule_id": "rule_default", "reason": "default", "policy_version": "p_v1"}

decision := {"decision": "ALLOW", "rule_id": "rule_allow", "reason": "ok", "policy_version": "p_v1"} if {
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
				Decision:      "ALLOW",
				RuleID:        "rule_allow",
				Reason:        "ok",
				PolicyVersion: "p_v1",
			},
		},
		{
			name:    "invalid policy output",
			policy:  models.Policy{PolicyID: "p2", TenantID: "t1", RegoModule: invalidPolicy, PolicyVersion: "p_v1"},
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
