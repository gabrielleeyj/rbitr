package public

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/connector"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/testhelpers"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

func TestConcurrentApprovalExecution(t *testing.T) {
	// Simulates two agents racing to execute the same approved request.
	// The DB WHERE status='APPROVED' clause ensures exactly one succeeds.
	cases := []struct {
		name                string
		firstReturnErr      error
		secondReturnErr     error
		expectedSuccessCode int
		expectedFailCode    int
		expectedFailBody    string
	}{
		{
			name:                "second agent gets invalid_state",
			firstReturnErr:      nil,
			secondReturnErr:     store.ErrInvalidState,
			expectedSuccessCode: http.StatusOK,
			expectedFailCode:    http.StatusConflict,
			expectedFailBody:    "approval_already_executed",
		},
		{
			name:                "second agent gets not_found after execution",
			firstReturnErr:      nil,
			secondReturnErr:     store.ErrNotFound,
			expectedSuccessCode: http.StatusOK,
			expectedFailCode:    http.StatusForbidden,
			expectedFailBody:    "approval_token_invalid",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tenant := models.Tenant{TenantID: "t1", Name: "Tenant", Enabled: true}
			tool := models.Tool{ToolID: "mock_internal", TenantID: "t1", BaseURL: "http://example", AuthType: "api_key", AuthValue: "key"}

			bodyHash := utils.HashBody([]byte("{}"))
			canonical := utils.CanonicalRequest{
				TenantID: "t1",
				AgentID:  "agent",
				ToolID:   "mock_internal",
				Method:   "POST",
				Path:     "/refund",
				Query:    "",
				Headers:  map[string]string{"content-type": "application/json"},
				BodyHash: bodyHash,
			}
			requestHash := utils.HashCanonical(&canonical)

			approval := models.ApprovalRequest{
				ApprovalRequestID: "ar1",
				TenantID:          "t1",
				AgentID:           "agent",
				ToolID:            "mock_internal",
				ActionType:        "PAYMENT.REFUND",
				RequestHash:       requestHash,
				Status:            "APPROVED",
				ApprovalTokenHash: utils.HashString("token123"),
				ExpiresAt:         time.Now().UTC().Add(10 * time.Minute),
				PolicyVersion:     "p_v1",
				ActionSummary:     "Refund",
				Risk:              "HIGH",
				RuleID:            "rule_approval",
			}

			// Track how many times execution claim succeeds.
			var executionCount atomic.Int32
			var callCount atomic.Int32

			storeMock := newPublicStoreMock(t)
			storeMock.On("GetTenantByKeyHash", mock.Anything, mock.Anything).Return(tenant, nil)
			storeMock.On("GetApprovalForExecution", mock.Anything, "t1", "ar1").Return(approval, nil)
			storeMock.On("GetTool", mock.Anything, "t1", "mock_internal").Return(tool, nil)
			storeMock.On("InsertADR", mock.Anything, mock.Anything).Return(nil)
			storeMock.On("ClaimApprovalExecution", mock.Anything, "t1", "ar1", utils.HashString("token123"), requestHash, mock.Anything).
				Return(func(_ context.Context, _, _, _, _ string, _ time.Time) error {
					n := callCount.Add(1)
					if n == 1 {
						executionCount.Add(1)
						return tc.firstReturnErr
					}
					return tc.secondReturnErr
				})
			storeMock.On("MarkApprovalExecuted", mock.Anything, "t1", "ar1", mock.Anything, mock.Anything, mock.Anything).
				Return(nil)

			connMock := connector.NewMockConnector(t)
			connMock.On("Execute", mock.Anything, mock.Anything).
				Return(connector.Response{Status: http.StatusOK, Headers: map[string]string{}, Body: []byte("ok"), BodyHash: "sha256:abc"}, nil)

			deps := &Dependencies{
				Store:     storeMock,
				Connector: connMock,
				Metrics:   newTestMetrics(),
				Config:    config.Config{BodyLimitSize: 256 * 1024, ResponseLimit: 256 * 1024},
			}

			payload := ToolCallRequest{
				HTTPMethod: "POST",
				Path:       "/refund",
				Query:      "",
				Headers:    map[string]string{"Content-Type": "application/json"},
				Body:       "{}",
			}

			var wg sync.WaitGroup
			var results [2]int

			for i := range 2 {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					ctx, req, rec := testhelpers.MakeRequestWithParams(
						http.MethodPost,
						testhelpers.MakeBody(payload),
						testhelpers.Params{Names: []string{"tool_id"}, Values: []string{"mock_internal"}},
					)
					req.Header.Set(auth.TenantKeyHeader, "key")
					req.Header.Set(auth.AgentIDHeader, "agent")
					req.Header.Set(approvalHeaderID, "ar1")
					req.Header.Set(approvalHeaderToken, "token123")

					_ = deps.handleToolCall(ctx)
					results[idx] = rec.Code
				}(i)
			}
			wg.Wait()

			// Exactly one should succeed with 200
			successCount := 0
			failCount := 0
			for _, code := range results {
				switch code {
				case tc.expectedSuccessCode:
					successCount++
				case tc.expectedFailCode:
					failCount++
				}
			}
			require.Equal(t, 1, successCount, "exactly one request should succeed with %d", tc.expectedSuccessCode)
			require.Equal(t, 1, failCount, "exactly one request should fail with %d", tc.expectedFailCode)
			require.Equal(t, int32(1), executionCount.Load(), "exactly one execution should complete")
		})
	}
}

func TestApprovalDoubleExecutionReturnsConflict(t *testing.T) {
	// Verifies that re-executing an already-executed approval returns 409.
	tenant := models.Tenant{TenantID: "t1", Name: "Tenant", Enabled: true}

	bodyHash := utils.HashBody([]byte("{}"))
	canonical := utils.CanonicalRequest{
		TenantID: "t1",
		AgentID:  "agent",
		ToolID:   "mock_internal",
		Method:   "POST",
		Path:     "/refund",
		Query:    "",
		Headers:  map[string]string{"content-type": "application/json"},
		BodyHash: bodyHash,
	}
	requestHash := utils.HashCanonical(&canonical)

	storeMock := newPublicStoreMock(t)
	storeMock.On("GetTenantByKeyHash", mock.Anything, mock.Anything).Return(tenant, nil)
	storeMock.On("GetApprovalForExecution", mock.Anything, "t1", "ar1").Return(models.ApprovalRequest{
		ApprovalRequestID: "ar1",
		TenantID:          "t1",
		AgentID:           "agent",
		ToolID:            "mock_internal",
		ActionType:        "PAYMENT.REFUND",
		RequestHash:       requestHash,
		Status:            "EXECUTED",
		ApprovalTokenHash: utils.HashString("token123"),
		ExpiresAt:         time.Now().UTC().Add(10 * time.Minute),
		PolicyVersion:     "p_v1",
		ActionSummary:     "Refund",
		Risk:              "HIGH",
		RuleID:            "rule_approval",
	}, nil)

	deps := &Dependencies{
		Store:   storeMock,
		Metrics: newTestMetrics(),
		Config:  config.Config{BodyLimitSize: 256 * 1024},
	}

	payload := ToolCallRequest{
		HTTPMethod: "POST",
		Path:       "/refund",
		Query:      "",
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       "{}",
	}
	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		testhelpers.MakeBody(payload),
		testhelpers.Params{Names: []string{"tool_id"}, Values: []string{"mock_internal"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "key")
	req.Header.Set(auth.AgentIDHeader, "agent")
	req.Header.Set(approvalHeaderID, "ar1")
	req.Header.Set(approvalHeaderToken, "token123")

	err := deps.handleToolCall(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestRetryWithinWindowReturnsConflictWhenAlreadyExecuted(t *testing.T) {
	// When an approval is in EXECUTING state but ExecutedAt is set (tool
	// succeeded, DB marked executed_at), a retry should get 409 conflict
	// instead of re-executing the tool.
	tenant := models.Tenant{TenantID: "t1", Name: "Tenant", Enabled: true}

	bodyHash := utils.HashBody([]byte("{}"))
	canonical := utils.CanonicalRequest{
		TenantID: "t1",
		AgentID:  "agent",
		ToolID:   "mock_internal",
		Method:   "POST",
		Path:     "/refund",
		Query:    "",
		Headers:  map[string]string{"content-type": "application/json"},
		BodyHash: bodyHash,
	}
	requestHash := utils.HashCanonical(&canonical)
	executedAt := time.Now().UTC().Add(-10 * time.Second)
	executingAt := time.Now().UTC().Add(-20 * time.Second)

	storeMock := newPublicStoreMock(t)
	storeMock.On("GetTenantByKeyHash", mock.Anything, mock.Anything).Return(tenant, nil)
	storeMock.On("GetApprovalForExecution", mock.Anything, "t1", "ar1").Return(models.ApprovalRequest{
		ApprovalRequestID: "ar1",
		TenantID:          "t1",
		AgentID:           "agent",
		ToolID:            "mock_internal",
		ActionType:        "PAYMENT.REFUND",
		RequestHash:       requestHash,
		Status:            "EXECUTING",
		ApprovalTokenHash: utils.HashString("token123"),
		ExpiresAt:         time.Now().UTC().Add(10 * time.Minute),
		ExecutingAt:       &executingAt,
		ExecutedAt:        &executedAt,
		PolicyVersion:     "p_v1",
		ActionSummary:     "Refund",
		Risk:              "HIGH",
		RuleID:            "rule_approval",
	}, nil)

	deps := &Dependencies{
		Store:   storeMock,
		Metrics: newTestMetrics(),
		Config:  config.Config{BodyLimitSize: 256 * 1024},
	}

	payload := ToolCallRequest{
		HTTPMethod: "POST",
		Path:       "/refund",
		Query:      "",
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       "{}",
	}
	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		testhelpers.MakeBody(payload),
		testhelpers.Params{Names: []string{"tool_id"}, Values: []string{"mock_internal"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "key")
	req.Header.Set(auth.AgentIDHeader, "agent")
	req.Header.Set(approvalHeaderID, "ar1")
	req.Header.Set(approvalHeaderToken, "token123")

	err := deps.handleToolCall(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestRetryWithinWindowAllowsReExecutionWhenNotCompleted(t *testing.T) {
	// When an approval is in EXECUTING state, ExecutedAt is nil, and the
	// retry is within the 60-second window, allow re-execution.
	tenant := models.Tenant{TenantID: "t1", Name: "Tenant", Enabled: true}
	tool := models.Tool{ToolID: "mock_internal", TenantID: "t1", BaseURL: "http://example", AuthType: "api_key", AuthValue: "key"}

	bodyHash := utils.HashBody([]byte("{}"))
	canonical := utils.CanonicalRequest{
		TenantID: "t1",
		AgentID:  "agent",
		ToolID:   "mock_internal",
		Method:   "POST",
		Path:     "/refund",
		Query:    "",
		Headers:  map[string]string{"content-type": "application/json"},
		BodyHash: bodyHash,
	}
	requestHash := utils.HashCanonical(&canonical)
	executingAt := time.Now().UTC().Add(-10 * time.Second)

	storeMock := newPublicStoreMock(t)
	storeMock.On("GetTenantByKeyHash", mock.Anything, mock.Anything).Return(tenant, nil)
	storeMock.On("GetApprovalForExecution", mock.Anything, "t1", "ar1").Return(models.ApprovalRequest{
		ApprovalRequestID: "ar1",
		TenantID:          "t1",
		AgentID:           "agent",
		ToolID:            "mock_internal",
		ActionType:        "PAYMENT.REFUND",
		RequestHash:       requestHash,
		Status:            "EXECUTING",
		ApprovalTokenHash: utils.HashString("token123"),
		ExpiresAt:         time.Now().UTC().Add(10 * time.Minute),
		ExecutingAt:       &executingAt,
		ExecutedAt:        nil,
		PolicyVersion:     "p_v1",
		ActionSummary:     "Refund",
		Risk:              "HIGH",
		RuleID:            "rule_approval",
		ExecutionID:       "ar1",
	}, nil)
	storeMock.On("GetTool", mock.Anything, "t1", "mock_internal").Return(tool, nil)
	storeMock.On("InsertADR", mock.Anything, mock.Anything).Return(nil)
	storeMock.On("MarkApprovalExecuted", mock.Anything, "t1", "ar1", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	connMock := connector.NewMockConnector(t)
	connMock.On("Execute", mock.Anything, mock.Anything).
		Return(connector.Response{Status: http.StatusOK, Headers: map[string]string{}, Body: []byte("ok"), BodyHash: "sha256:abc"}, nil)

	deps := &Dependencies{
		Store:     storeMock,
		Connector: connMock,
		Metrics:   newTestMetrics(),
		Config:    config.Config{BodyLimitSize: 256 * 1024, ResponseLimit: 256 * 1024},
	}

	payload := ToolCallRequest{
		HTTPMethod: "POST",
		Path:       "/refund",
		Query:      "",
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       "{}",
	}
	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		testhelpers.MakeBody(payload),
		testhelpers.Params{Names: []string{"tool_id"}, Values: []string{"mock_internal"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "key")
	req.Header.Set(auth.AgentIDHeader, "agent")
	req.Header.Set(approvalHeaderID, "ar1")
	req.Header.Set(approvalHeaderToken, "token123")

	err := deps.handleToolCall(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	connMock.AssertCalled(t, "Execute", mock.Anything, mock.Anything)
}

func TestConcurrentRetryOnlyOneExecutes(t *testing.T) {
	// Two goroutines retry the same EXECUTING approval within the retry
	// window. Only one should succeed through to MarkApprovalExecuted;
	// the other should fail because MarkApprovalExecuted returns
	// ErrInvalidState (due to the AND executed_at IS NULL guard).
	tenant := models.Tenant{TenantID: "t1", Name: "Tenant", Enabled: true}
	tool := models.Tool{ToolID: "mock_internal", TenantID: "t1", BaseURL: "http://example", AuthType: "api_key", AuthValue: "key"}

	bodyHash := utils.HashBody([]byte("{}"))
	canonical := utils.CanonicalRequest{
		TenantID: "t1",
		AgentID:  "agent",
		ToolID:   "mock_internal",
		Method:   "POST",
		Path:     "/refund",
		Query:    "",
		Headers:  map[string]string{"content-type": "application/json"},
		BodyHash: bodyHash,
	}
	requestHash := utils.HashCanonical(&canonical)
	executingAt := time.Now().UTC().Add(-10 * time.Second)

	var markCount atomic.Int32

	storeMock := newPublicStoreMock(t)
	storeMock.On("GetTenantByKeyHash", mock.Anything, mock.Anything).Return(tenant, nil)
	storeMock.On("GetApprovalForExecution", mock.Anything, "t1", "ar1").Return(models.ApprovalRequest{
		ApprovalRequestID: "ar1",
		TenantID:          "t1",
		AgentID:           "agent",
		ToolID:            "mock_internal",
		ActionType:        "PAYMENT.REFUND",
		RequestHash:       requestHash,
		Status:            "EXECUTING",
		ApprovalTokenHash: utils.HashString("token123"),
		ExpiresAt:         time.Now().UTC().Add(10 * time.Minute),
		ExecutingAt:       &executingAt,
		ExecutedAt:        nil,
		PolicyVersion:     "p_v1",
		ActionSummary:     "Refund",
		Risk:              "HIGH",
		RuleID:            "rule_approval",
		ExecutionID:       "ar1",
	}, nil)
	storeMock.On("GetTool", mock.Anything, "t1", "mock_internal").Return(tool, nil)
	storeMock.On("InsertADR", mock.Anything, mock.Anything).Return(nil)
	storeMock.On("MarkApprovalExecuted", mock.Anything, "t1", "ar1", mock.Anything, mock.Anything, mock.Anything).
		Return(func(_ context.Context, _, _, _, _ string, _ time.Time) error {
			n := markCount.Add(1)
			if n == 1 {
				return nil
			}
			return store.ErrInvalidState
		})

	connMock := connector.NewMockConnector(t)
	connMock.On("Execute", mock.Anything, mock.Anything).
		Return(connector.Response{Status: http.StatusOK, Headers: map[string]string{}, Body: []byte("ok"), BodyHash: "sha256:abc"}, nil)

	deps := &Dependencies{
		Store:     storeMock,
		Connector: connMock,
		Metrics:   newTestMetrics(),
		Config:    config.Config{BodyLimitSize: 256 * 1024, ResponseLimit: 256 * 1024},
	}

	payload := ToolCallRequest{
		HTTPMethod: "POST",
		Path:       "/refund",
		Query:      "",
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       "{}",
	}

	var wg sync.WaitGroup
	var results [2]int

	for i := range 2 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPost,
				testhelpers.MakeBody(payload),
				testhelpers.Params{Names: []string{"tool_id"}, Values: []string{"mock_internal"}},
			)
			req.Header.Set(auth.TenantKeyHeader, "key")
			req.Header.Set(auth.AgentIDHeader, "agent")
			req.Header.Set(approvalHeaderID, "ar1")
			req.Header.Set(approvalHeaderToken, "token123")

			_ = deps.handleToolCall(ctx)
			results[idx] = rec.Code
		}(i)
	}
	wg.Wait()

	successCount := 0
	for _, code := range results {
		if code == http.StatusOK {
			successCount++
		}
	}
	// At least one succeeds. The second may succeed (if it finishes before
	// the first calls MarkApprovalExecuted) or get a 500 (ErrInvalidState
	// from MarkApprovalExecuted). Both goroutines execute the tool, but
	// only one can mark it as executed.
	require.GreaterOrEqual(t, successCount, 1, "at least one request should succeed")
	require.LessOrEqual(t, successCount, 2, "at most two requests should succeed")
}
