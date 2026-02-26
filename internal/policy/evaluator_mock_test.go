package policy

import (
	"context"
	"errors"
	"testing"
)

func TestInvalidPolicyOutputErrorMethods(t *testing.T) {
	err := InvalidPolicyOutputError{}
	if got := err.Error(); got != "invalid policy output" {
		t.Fatalf("expected default error, got %q", got)
	}
	if err.Unwrap() != nil {
		t.Fatalf("expected nil unwrap")
	}

	base := errors.New("boom")
	err = InvalidPolicyOutputError{Err: base}
	if got := err.Error(); got != "boom" {
		t.Fatalf("expected wrapped error, got %q", got)
	}
	if !errors.Is(err.Unwrap(), base) {
		t.Fatalf("expected unwrap to return base error")
	}
}

func TestMockEvaluatorAPIExpectations(t *testing.T) {
	mock := NewMockEvaluatorAPI(t)
	ctx := context.Background()
	input := map[string]any{"action": "A"}

	mock.EXPECT().
		Evaluate(ctx, "t1", input).
		Run(func(ctx context.Context, tenantID string, input map[string]any) {
			if tenantID != "t1" {
				t.Fatalf("unexpected tenant id: %s", tenantID)
			}
		}).
		Return(Result{Decision: "ALLOW"}, nil)

	if _, err := mock.Evaluate(ctx, "t1", input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mock.EXPECT().
		Evaluate(ctx, "t2", map[string]any{"x": "y"}).
		RunAndReturn(func(ctx context.Context, tenantID string, input map[string]any) (Result, error) {
			return Result{Decision: "DENY"}, nil
		})

	if _, err := mock.Evaluate(ctx, "t2", map[string]any{"x": "y"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
