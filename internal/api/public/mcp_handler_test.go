package public

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/mcp"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/policy"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
	"github.com/gabrielleeyj/rbitr/internal/testhelpers"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

func TestHandleMCP_Authentication(t *testing.T) {
	tests := []struct {
		name           string
		tenantKey      string
		agentID        string
		tenantID       string
		setupMock      func(*store.MockStoreAPI)
		expectedError  bool
		expectedErrMsg string
	}{
		{
			name:      "missing tenant key",
			tenantKey: "",
			agentID:   "agent1",
			tenantID:  "t_demo",
			setupMock: func(m *store.MockStoreAPI) {
				// No mock expectations needed - auth fails before store call
			},
			expectedError:  true,
			expectedErrMsg: "authentication failed",
		},
		{
			name:      "missing agent id",
			tenantKey: "valid_key",
			agentID:   "",
			tenantID:  "t_demo",
			setupMock: func(m *store.MockStoreAPI) {
				// No mock expectations needed - auth fails before store call
			},
			expectedError:  true,
			expectedErrMsg: "authentication failed",
		},
		{
			name:      "invalid tenant key",
			tenantKey: "invalid_key",
			agentID:   "agent1",
			tenantID:  "t_demo",
			setupMock: func(m *store.MockStoreAPI) {
				m.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
					Return(models.Tenant{}, store.ErrNotFound)
			},
			expectedError:  true,
			expectedErrMsg: "authentication failed",
		},
		{
			name:      "tenant id mismatch",
			tenantKey: "valid_key",
			agentID:   "agent1",
			tenantID:  "t_wrong",
			setupMock: func(m *store.MockStoreAPI) {
				m.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
					Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
			},
			expectedError:  true,
			expectedErrMsg: "authentication failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := newPublicStoreMock(t)
			tt.setupMock(mockStore)

			deps := &Dependencies{
				Store:   mockStore,
				Metrics: newTestMetrics(),
			}

			// Create valid JSON-RPC request
			reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPost,
				bytes.NewReader([]byte(reqBody)),
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{tt.tenantID}},
			)
			req.Header.Set(auth.TenantKeyHeader, tt.tenantKey)
			req.Header.Set(auth.AgentIDHeader, tt.agentID)

			err := deps.handleMCP(ctx)

			// Handler returns nil on success (error is in response body)
			assert.NoError(t, err)
			assert.Equal(t, http.StatusOK, rec.Code)

			// Parse JSON-RPC response
			var resp mcp.Response
			err = json.Unmarshal(rec.Body.Bytes(), &resp)
			require.NoError(t, err)

			if tt.expectedError {
				assert.NotNil(t, resp.Error, "Expected error in JSON-RPC response")
				if resp.Error != nil {
					assert.Contains(t, resp.Error.Message, tt.expectedErrMsg)
				}
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleMCP_ValidRequest(t *testing.T) {
	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
	mockStore.On("ListTools", mock.Anything, "t_demo", false, mock.Anything).Return([]models.Tool{
		{
			ToolID:          "test_tool",
			TenantID:        "t_demo",
			BaseURL:         "http://localhost:8090",
			Description:     "Test tool",
			InputSchemaJSON: []byte(`{"type":"object"}`),
		},
	}, nil)

	deps := &Dependencies{
		Store:   mockStore,
		Metrics: telemetry.NewMetrics(),
	}

	reqBody := `{"jsonrpc":"2.0","id":"req-1","method":"tools/list","params":{}}`

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		bytes.NewReader([]byte(reqBody)),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")

	err := deps.handleMCP(ctx)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Parse JSON-RPC response
	var resp mcp.Response
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "2.0", resp.JSONRPC)
	assert.NotNil(t, resp.ID)
	// tools/list is now implemented and should succeed
	assert.Nil(t, resp.Error)
	assert.NotNil(t, resp.Result)

	mockStore.AssertExpectations(t)
}

func TestHandleMCP_InvalidJSONRPC(t *testing.T) {
	tests := []struct {
		name            string
		body            string
		expectedErrCode int
		expectedID      *mcp.RequestID // Expected ID in error response
	}{
		{
			name:            "invalid json",
			body:            `{invalid json`,
			expectedErrCode: mcp.ErrorParseError,
			expectedID:      nil, // Cannot extract ID from invalid JSON
		},
		{
			name:            "missing jsonrpc field",
			body:            `{"id":1,"method":"test"}`,
			expectedErrCode: mcp.ErrorInvalidRequest,
			expectedID:      mcp.NewNumberID(1), // ID should be preserved
		},
		{
			name:            "wrong jsonrpc version",
			body:            `{"jsonrpc":"1.0","id":123,"method":"test"}`,
			expectedErrCode: mcp.ErrorInvalidRequest,
			expectedID:      mcp.NewNumberID(123), // ID should be preserved
		},
		{
			name:            "missing method",
			body:            `{"jsonrpc":"2.0","id":"client-req-456"}`,
			expectedErrCode: mcp.ErrorInvalidRequest,
			expectedID:      mcp.NewStringID("client-req-456"), // ID should be preserved
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := newPublicStoreMock(t)
			mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
				Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)

			deps := &Dependencies{
				Store:   mockStore,
				Metrics: newTestMetrics(),
			}

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPost,
				bytes.NewReader([]byte(tt.body)),
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
			)
			req.Header.Set(auth.TenantKeyHeader, "valid_key")
			req.Header.Set(auth.AgentIDHeader, "agent1")

			err := deps.handleMCP(ctx)
			assert.NoError(t, err)
			assert.Equal(t, http.StatusOK, rec.Code)

			// Parse JSON-RPC response
			var resp mcp.Response
			err = json.Unmarshal(rec.Body.Bytes(), &resp)
			require.NoError(t, err)

			assert.NotNil(t, resp.Error)
			assert.Equal(t, tt.expectedErrCode, resp.Error.Code)

			// Verify ID preservation in error responses
			if tt.expectedID != nil {
				require.NotNil(t, resp.ID, "ID should be preserved in error response")
				if tt.expectedID.String() != nil {
					require.NotNil(t, resp.ID.String())
					assert.Equal(t, *tt.expectedID.String(), *resp.ID.String())
				} else if tt.expectedID.Number() != nil {
					require.NotNil(t, resp.ID.Number())
					assert.Equal(t, *tt.expectedID.Number(), *resp.ID.Number())
				}
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleMCP_MethodRouting(t *testing.T) {
	tests := []struct {
		name            string
		method          string
		params          string
		setupMock       func(*store.MockStoreAPI)
		expectedErrCode int
	}{
		{
			name:   "tools/call with missing tool name",
			method: "tools/call",
			params: `{}`,
			setupMock: func(m *store.MockStoreAPI) {
				// No extra mocks needed
			},
			expectedErrCode: mcp.ErrorInvalidParams,
		},
		{
			name:   "unknown method with no MCP upstream",
			method: "unknown/method",
			params: `{}`,
			setupMock: func(m *store.MockStoreAPI) {
				m.On("GetTenantConfig", mock.Anything, "t_demo").
					Return(models.TenantConfig{}, store.ErrNotFound)
				// Pass-through needs ListTools to find upstream - return no MCP tools
				m.On("ListTools", mock.Anything, "t_demo", false, mock.Anything).Return([]models.Tool{}, nil)
			},
			expectedErrCode: mcp.ErrorMethodNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := newPublicStoreMock(t)
			mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
				Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
			tt.setupMock(mockStore)

			deps := &Dependencies{
				Store:   mockStore,
				Metrics: newTestMetrics(),
			}

			reqBody := `{"jsonrpc":"2.0","id":1,"method":"` + tt.method + `","params":` + tt.params + `}`

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPost,
				bytes.NewReader([]byte(reqBody)),
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
			)
			req.Header.Set(auth.TenantKeyHeader, "valid_key")
			req.Header.Set(auth.AgentIDHeader, "agent1")

			err := deps.handleMCP(ctx)
			assert.NoError(t, err)
			assert.Equal(t, http.StatusOK, rec.Code)

			// Parse JSON-RPC response
			var resp mcp.Response
			err = json.Unmarshal(rec.Body.Bytes(), &resp)
			require.NoError(t, err)

			assert.NotNil(t, resp.Error)
			assert.Equal(t, tt.expectedErrCode, resp.Error.Code)

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleMCP_ToolsList(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func(*store.MockStoreAPI)
		validateTools func(*testing.T, []mcp.Tool)
	}{
		{
			name: "returns tools with descriptions and schemas",
			setupMock: func(m *store.MockStoreAPI) {
				m.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
					Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
				m.On("ListTools", mock.Anything, "t_demo", false, mock.Anything).Return([]models.Tool{
					{
						ToolID:          "jira",
						TenantID:        "t_demo",
						BaseURL:         "http://localhost:8081",
						Description:     "Jira integration for issue management",
						InputSchemaJSON: []byte(`{"type":"object","properties":{"action":{"type":"string"}},"required":["action"]}`),
					},
					{
						ToolID:          "mock_internal",
						TenantID:        "t_demo",
						BaseURL:         "http://localhost:8090",
						Description:     "Internal mock tool for testing",
						InputSchemaJSON: []byte(`{"type":"object","additionalProperties":true}`),
					},
				}, nil)
			},
			validateTools: func(t *testing.T, tools []mcp.Tool) {
				require.Len(t, tools, 2)

				// Check first tool
				assert.Equal(t, "jira", tools[0].Name)
				assert.Equal(t, "Jira integration for issue management", tools[0].Description)
				assert.NotEmpty(t, tools[0].InputSchema)

				// Check second tool
				assert.Equal(t, "mock_internal", tools[1].Name)
				assert.Equal(t, "Internal mock tool for testing", tools[1].Description)
				assert.NotEmpty(t, tools[1].InputSchema)
			},
		},
		{
			name: "provides defaults for missing description and schema",
			setupMock: func(m *store.MockStoreAPI) {
				m.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
					Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
				m.On("ListTools", mock.Anything, "t_demo", false, mock.Anything).Return([]models.Tool{
					{
						ToolID:   "minimal_tool",
						TenantID: "t_demo",
						BaseURL:  "http://localhost:9000",
						// No description or schema
					},
				}, nil)
			},
			validateTools: func(t *testing.T, tools []mcp.Tool) {
				require.Len(t, tools, 1)
				assert.Equal(t, "minimal_tool", tools[0].Name)
				assert.Equal(t, "No description available", tools[0].Description)
				assert.Equal(t, `{"type":"object","additionalProperties":true}`, string(tools[0].InputSchema))
			},
		},
		{
			name: "returns empty list when no tools",
			setupMock: func(m *store.MockStoreAPI) {
				m.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
					Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
				m.On("ListTools", mock.Anything, "t_demo", false, mock.Anything).Return([]models.Tool{}, nil)
			},
			validateTools: func(t *testing.T, tools []mcp.Tool) {
				assert.Empty(t, tools)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := newPublicStoreMock(t)
			tt.setupMock(mockStore)

			deps := &Dependencies{
				Store:   mockStore,
				Metrics: newTestMetrics(),
			}

			reqBody := `{"jsonrpc":"2.0","id":"req-list-1","method":"tools/list","params":{}}`

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPost,
				bytes.NewReader([]byte(reqBody)),
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
			)
			req.Header.Set(auth.TenantKeyHeader, "valid_key")
			req.Header.Set(auth.AgentIDHeader, "agent1")

			err := deps.handleMCP(ctx)
			assert.NoError(t, err)
			assert.Equal(t, http.StatusOK, rec.Code)

			// Parse JSON-RPC response
			var resp mcp.Response
			err = json.Unmarshal(rec.Body.Bytes(), &resp)
			require.NoError(t, err)

			// Should be success response
			assert.Nil(t, resp.Error)
			assert.NotNil(t, resp.Result)

			// Parse result
			var result mcp.ToolsListResult
			err = json.Unmarshal(resp.Result, &result)
			require.NoError(t, err)

			// Validate tools
			if tt.validateTools != nil {
				tt.validateTools(t, result.Tools)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleMCP_ToolsList_StoreError(t *testing.T) {
	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
	mockStore.On("ListTools", mock.Anything, "t_demo", false, mock.Anything).
		Return([]models.Tool{}, assert.AnError)

	deps := &Dependencies{
		Store:   mockStore,
		Metrics: newTestMetrics(),
	}

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		bytes.NewReader([]byte(reqBody)),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")

	err := deps.handleMCP(ctx)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Parse JSON-RPC response
	var resp mcp.Response
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	// Should return error
	assert.NotNil(t, resp.Error)
	assert.Equal(t, mcp.ErrorInternalError, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "failed to list tools")

	mockStore.AssertExpectations(t)
}

func TestHandleMCP_RequestIDPreservation(t *testing.T) {
	tests := []struct {
		name      string
		requestID string
		checkIDFn func(*testing.T, *mcp.RequestID)
	}{
		{
			name:      "string id",
			requestID: `"test-id"`,
			checkIDFn: func(t *testing.T, id *mcp.RequestID) {
				require.NotNil(t, id)
				assert.NotNil(t, id.String())
				assert.Equal(t, "test-id", *id.String())
			},
		},
		{
			name:      "number id",
			requestID: `42`,
			checkIDFn: func(t *testing.T, id *mcp.RequestID) {
				require.NotNil(t, id)
				assert.NotNil(t, id.Number())
				assert.Equal(t, float64(42), *id.Number())
			},
		},
		{
			name:      "null id (invalid request)",
			requestID: `null`,
			checkIDFn: func(t *testing.T, id *mcp.RequestID) {
				assert.Nil(t, id)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := newPublicStoreMock(t)
			mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
				Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
			mockStore.On("ListTools", mock.Anything, "t_demo", false, mock.Anything).Maybe().Return([]models.Tool{}, nil)

			deps := &Dependencies{
				Store:   mockStore,
				Metrics: newTestMetrics(),
			}

			reqBody := `{"jsonrpc":"2.0","id":` + tt.requestID + `,"method":"tools/list","params":{}}`

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPost,
				bytes.NewReader([]byte(reqBody)),
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
			)
			req.Header.Set(auth.TenantKeyHeader, "valid_key")
			req.Header.Set(auth.AgentIDHeader, "agent1")

			err := deps.handleMCP(ctx)
			assert.NoError(t, err)

			// For invalid null ID, expect JSON-RPC error response
			if tt.requestID == `null` {
				assert.Equal(t, http.StatusOK, rec.Code)
				var resp mcp.Response
				err = json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.NotNil(t, resp.Error)
				assert.Equal(t, mcp.ErrorInvalidRequest, resp.Error.Code)
				assert.Nil(t, resp.ID)
				mockStore.AssertExpectations(t)
				return
			}

			var resp mcp.Response
			err = json.Unmarshal(rec.Body.Bytes(), &resp)
			require.NoError(t, err)

			tt.checkIDFn(t, resp.ID)

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleMCP_ToolsCall_ApprovalResubmitRequiresApprovalRequestID(t *testing.T) {
	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
	mockStore.On("GetTool", mock.Anything, "t_demo", "jira").
		Return(models.Tool{ToolID: "jira", TenantID: "t_demo", Transport: "mcp_streamable_http", MCPUpstreamURL: "http://upstream-mcp.local"}, nil)

	deps := &Dependencies{
		Store:   mockStore,
		Metrics: newTestMetrics(),
	}

	reqBody := `{
		"jsonrpc":"2.0",
		"id":1,
		"method":"tools/call",
		"params":{
			"name":"jira",
			"arguments":{
				"action":"issue_create",
				"_rbitr_approval_token":"apt_123"
			}
		}
	}`

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		bytes.NewReader([]byte(reqBody)),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")

	err := deps.handleMCP(ctx)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp mcp.Response
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.NotNil(t, resp.Error)
	assert.Equal(t, mcp.ErrorInvalidParams, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "_rbitr_approval_request_id is required")
	mockStore.AssertNotCalled(t, "ListApprovalRequests", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockStore.AssertExpectations(t)
}

func TestHandleMCP_ToolsCall_ResubmitIgnoresInternalApprovalFieldsInHash(t *testing.T) {
	// Create mock upstream MCP server
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the request to get the ID
		var req mcp.Request
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		require.Equal(t, "tools/call", req.Method)
		var params mcp.ToolsCallParams
		err = json.Unmarshal(req.Params, &params)
		require.NoError(t, err)
		var args map[string]any
		err = json.Unmarshal(params.Arguments, &args)
		require.NoError(t, err)
		_, hasToken := args["_rbitr_approval_token"]
		_, hasApprovalID := args["_rbitr_approval_request_id"]
		assert.False(t, hasToken, "approval token must not be forwarded upstream")
		assert.False(t, hasApprovalID, "approval request id must not be forwarded upstream")

		resultData, _ := json.Marshal(mcp.ToolsCallResult{
			Content: []mcp.Content{
				{Type: "text", Text: "Success"},
			},
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(mcp.Response{
			JSONRPC: "2.0",
			ID:      req.ID, // Echo back the request ID
			Result:  resultData,
		})
		require.NoError(t, err)
	}))
	defer upstreamServer.Close()

	argumentsForHash := map[string]any{"action": "issue_create"}
	argumentsJSON, err := json.Marshal(argumentsForHash)
	require.NoError(t, err)

	canonical := utils.CanonicalRequest{
		TenantID: "t_demo",
		AgentID:  "agent1",
		ToolID:   "jira",
		Method:   "MCP_CALL",
		Path:     "/tools/call",
		Headers:  map[string]string{},
		BodyHash: utils.HashBody(argumentsJSON),
	}
	expectedHash := utils.HashCanonical(&canonical)

	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
	mockStore.On("GetTool", mock.Anything, "t_demo", "jira").
		Return(models.Tool{ToolID: "jira", TenantID: "t_demo", Transport: "mcp_streamable_http", MCPUpstreamURL: upstreamServer.URL}, nil)
	mockStore.On("GetApprovalForExecution", mock.Anything, "t_demo", "ar_123").
		Return(models.ApprovalRequest{
			ApprovalRequestID: "ar_123",
			TenantID:          "t_demo",
			Status:            "APPROVED",
			RequestHash:       expectedHash,
			ApprovalTokenHash: utils.HashString("apt_123"),
			ExpiresAt:         time.Now().UTC().Add(time.Hour),
			ActionType:        "MCP.jira",
			Risk:              "MEDIUM",
			ActionSummary:     "MCP tool call: jira",
			PolicyVersion:     "p_v1",
			RuleID:            "rule_approve",
		}, nil)
	mockStore.On("ClaimApprovalExecution", mock.Anything, "t_demo", "ar_123", utils.HashString("apt_123"), expectedHash, mock.Anything).
		Return(nil)
	mockStore.On("MarkApprovalExecuted", mock.Anything, "t_demo", "ar_123", mock.Anything, mock.Anything, mock.Anything).
		Return(nil)
	mockStore.On("InsertADR", mock.Anything, mock.MatchedBy(func(adr *models.ActionDecisionRecord) bool {
		return adr != nil && adr.ApprovalRequestID == "ar_123" && adr.RequestHash == expectedHash
	})).Return(nil)

	deps := &Dependencies{
		Store:   mockStore,
		Metrics: newTestMetrics(),
	}

	reqBody := `{
		"jsonrpc":"2.0",
		"id":1,
		"method":"tools/call",
		"params":{
			"name":"jira",
			"arguments":{
				"action":"issue_create",
				"_rbitr_approval_token":"apt_123",
				"_rbitr_approval_request_id":"ar_123"
			}
		}
	}`

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		bytes.NewReader([]byte(reqBody)),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")

	err = deps.handleMCP(ctx)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp mcp.Response
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Nil(t, resp.Error)
	assert.NotNil(t, resp.Result)
	mockStore.AssertExpectations(t)
}

func TestHandleMCP_ToolsCall_ResubmitReturnsErrorWhenADRPersistFails(t *testing.T) {
	// Create mock upstream MCP server
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the request to get the ID
		var req mcp.Request
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		resultData, err := json.Marshal(mcp.ToolsCallResult{
			Content: []mcp.Content{
				{Type: "text", Text: "Success"},
			},
		})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(mcp.Response{
			JSONRPC: "2.0",
			ID:      req.ID, // Echo back the request ID
			Result:  resultData,
		})
		require.NoError(t, err)
	}))
	defer upstreamServer.Close()

	argumentsForHash := map[string]any{"action": "issue_create"}
	argumentsJSON, err := json.Marshal(argumentsForHash)
	require.NoError(t, err)

	canonical := utils.CanonicalRequest{
		TenantID: "t_demo",
		AgentID:  "agent1",
		ToolID:   "jira",
		Method:   "MCP_CALL",
		Path:     "/tools/call",
		Headers:  map[string]string{},
		BodyHash: utils.HashBody(argumentsJSON),
	}
	expectedHash := utils.HashCanonical(&canonical)

	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
	mockStore.On("GetTool", mock.Anything, "t_demo", "jira").
		Return(models.Tool{ToolID: "jira", TenantID: "t_demo", Transport: "mcp_streamable_http", MCPUpstreamURL: upstreamServer.URL}, nil)
	mockStore.On("GetApprovalForExecution", mock.Anything, "t_demo", "ar_123").
		Return(models.ApprovalRequest{
			ApprovalRequestID: "ar_123",
			TenantID:          "t_demo",
			Status:            "APPROVED",
			RequestHash:       expectedHash,
			ApprovalTokenHash: utils.HashString("apt_123"),
			ExpiresAt:         time.Now().UTC().Add(time.Hour),
			ActionType:        "MCP.jira",
			Risk:              "MEDIUM",
			ActionSummary:     "MCP tool call: jira",
		}, nil)
	mockStore.On("ClaimApprovalExecution", mock.Anything, "t_demo", "ar_123", utils.HashString("apt_123"), expectedHash, mock.Anything).
		Return(nil)
	mockStore.On("MarkApprovalExecuted", mock.Anything, "t_demo", "ar_123", mock.Anything, mock.Anything, mock.Anything).
		Return(nil)
	mockStore.On("InsertADR", mock.Anything, mock.Anything).Return(assert.AnError)

	deps := &Dependencies{
		Store:   mockStore,
		Metrics: newTestMetrics(),
	}

	reqBody := `{
		"jsonrpc":"2.0",
		"id":1,
		"method":"tools/call",
		"params":{
			"name":"jira",
			"arguments":{
				"action":"issue_create",
				"_rbitr_approval_token":"apt_123",
				"_rbitr_approval_request_id":"ar_123"
			}
		}
	}`

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		bytes.NewReader([]byte(reqBody)),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")

	err = deps.handleMCP(ctx)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp mcp.Response
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	// When ADR persistence fails but tool execution succeeds, we return success
	// because the approval is already consumed and the tool was already executed.
	// The ADR failure is logged but not returned to the user.
	assert.Nil(t, resp.Error)
	assert.NotNil(t, resp.Result)
	mockStore.AssertExpectations(t)
}

func TestHandleMCP_NotificationWithoutIDReturns202(t *testing.T) {
	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
	mockStore.On("ListTools", mock.Anything, "t_demo", false, mock.Anything).Return([]models.Tool{}, nil)

	deps := &Dependencies{
		Store:   mockStore,
		Metrics: newTestMetrics(),
	}

	reqBody := `{"jsonrpc":"2.0","method":"tools/list","params":{}}`

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		bytes.NewReader([]byte(reqBody)),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")

	err := deps.handleMCP(ctx)
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.String())
	mockStore.AssertExpectations(t)
}

func TestHandleMCP_InitializeAndInitializedFlow(t *testing.T) {
	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)

	deps := &Dependencies{
		Store:   mockStore,
		Metrics: newTestMetrics(),
	}

	// initialize request
	initBody := `{
		"jsonrpc":"2.0",
		"id":"init-1",
		"method":"initialize",
		"params":{
			"protocolVersion":"2025-11-25",
			"capabilities":{},
			"clientInfo":{"name":"test-client","version":"1.0.0"}
		}
	}`

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		bytes.NewReader([]byte(initBody)),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")
	err := deps.handleMCP(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	var initResp mcp.Response
	err = json.Unmarshal(rec.Body.Bytes(), &initResp)
	require.NoError(t, err)
	require.Nil(t, initResp.Error)

	var initResult mcp.InitializeResult
	err = json.Unmarshal(initResp.Result, &initResult)
	require.NoError(t, err)
	assert.Equal(t, mcp.ProtocolVersion20251125, initResult.ProtocolVersion)
	assert.Equal(t, "rbitr-gateway", initResult.ServerInfo.Name)

	// notifications/initialized should be accepted as notification (no response body)
	initializedBody := `{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`
	ctx2, req2, rec2 := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		bytes.NewReader([]byte(initializedBody)),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req2.Header.Set(auth.TenantKeyHeader, "valid_key")
	req2.Header.Set(auth.AgentIDHeader, "agent1")
	err = deps.handleMCP(ctx2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, rec2.Code)
	assert.Empty(t, rec2.Body.String())

	mockStore.AssertExpectations(t)
}

func TestHandleMCPStream_GetEndpoint(t *testing.T) {
	originalDuration := mcpStreamMaxDuration
	originalHeartbeat := mcpStreamHeartbeatInterval
	originalMaxBytes := mcpStreamMaxBytes
	mcpStreamMaxDuration = 35 * time.Millisecond
	mcpStreamHeartbeatInterval = 10 * time.Millisecond
	mcpStreamMaxBytes = 4 * 1024
	t.Cleanup(func() {
		mcpStreamMaxDuration = originalDuration
		mcpStreamHeartbeatInterval = originalHeartbeat
		mcpStreamMaxBytes = originalMaxBytes
	})

	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)

	deps := &Dependencies{
		Store:   mockStore,
		Metrics: newTestMetrics(),
	}

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodGet,
		bytes.NewReader(nil),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")

	err := deps.handleMCPStream(ctx)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")
	assert.Contains(t, rec.Body.String(), ": connected")
	assert.Contains(t, rec.Body.String(), ": heartbeat")
	assert.Contains(t, rec.Body.String(), `"max_duration_reached"`)
	mockStore.AssertExpectations(t)
}

func TestHandleMCPStream_ByteLimit(t *testing.T) {
	originalDuration := mcpStreamMaxDuration
	originalHeartbeat := mcpStreamHeartbeatInterval
	originalMaxBytes := mcpStreamMaxBytes
	mcpStreamMaxDuration = 2 * time.Second
	mcpStreamHeartbeatInterval = 10 * time.Millisecond
	mcpStreamMaxBytes = int64(len(": connected\n\n"))
	t.Cleanup(func() {
		mcpStreamMaxDuration = originalDuration
		mcpStreamHeartbeatInterval = originalHeartbeat
		mcpStreamMaxBytes = originalMaxBytes
	})

	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)

	deps := &Dependencies{
		Store:   mockStore,
		Metrics: newTestMetrics(),
	}

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodGet,
		bytes.NewReader(nil),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")

	err := deps.handleMCPStream(ctx)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), ": connected")
	assert.NotContains(t, rec.Body.String(), ": heartbeat")
	assert.NotContains(t, rec.Body.String(), `"max_duration_reached"`)
	mockStore.AssertExpectations(t)
}

func TestHandleMCP_PassThrough_WithUpstream(t *testing.T) {
	// Create mock upstream MCP server that echoes back a result for resources/list
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req mcp.Request
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "resources/list", req.Method)

		resultData, err := json.Marshal(map[string]any{
			"resources": []any{},
		})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(mcp.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  resultData,
		})
		require.NoError(t, err)
	}))
	defer upstreamServer.Close()

	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
	mockStore.On("GetTenantConfig", mock.Anything, "t_demo").
		Return(models.TenantConfig{}, store.ErrNotFound)
	mockStore.On("ListTools", mock.Anything, "t_demo", false, mock.Anything).Return([]models.Tool{
		{
			ToolID:         "mcp_tool",
			TenantID:       "t_demo",
			Transport:      "mcp_streamable_http",
			MCPUpstreamURL: upstreamServer.URL,
		},
	}, nil)

	deps := &Dependencies{
		Store:   mockStore,
		Metrics: newTestMetrics(),
	}

	reqBody := `{"jsonrpc":"2.0","id":"pt-1","method":"resources/list","params":{}}`

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		bytes.NewReader([]byte(reqBody)),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")

	err := deps.handleMCP(ctx)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp mcp.Response
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Nil(t, resp.Error)
	assert.NotNil(t, resp.Result)
	assert.NotNil(t, resp.ID)
	require.NotNil(t, resp.ID.String())
	assert.Equal(t, "pt-1", *resp.ID.String())

	mockStore.AssertExpectations(t)
}

func TestHandleMCP_PassThrough_UsesConfiguredUpstreamTool(t *testing.T) {
	selectedUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req mcp.Request
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		resultData, err := json.Marshal(map[string]any{"source": "selected"})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(mcp.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  resultData,
		})
		require.NoError(t, err)
	}))
	defer selectedUpstream.Close()

	fallbackUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req mcp.Request
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		resultData, _ := json.Marshal(map[string]any{"source": "fallback"})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(mcp.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  resultData,
		})
		require.NoError(t, err)
	}))
	defer fallbackUpstream.Close()

	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
	mockStore.On("GetTenantConfig", mock.Anything, "t_demo").
		Return(models.TenantConfig{
			TenantID:                     "t_demo",
			MCPPassthroughUpstreamToolID: "z_selected",
		}, nil)
	// Order should not matter when explicit selection is configured.
	mockStore.On("ListTools", mock.Anything, "t_demo", false, mock.Anything).Return([]models.Tool{
		{
			ToolID:         "a_fallback",
			TenantID:       "t_demo",
			Transport:      "mcp_streamable_http",
			MCPUpstreamURL: fallbackUpstream.URL,
		},
		{
			ToolID:         "z_selected",
			TenantID:       "t_demo",
			Transport:      "mcp_streamable_http",
			MCPUpstreamURL: selectedUpstream.URL,
		},
	}, nil)

	deps := &Dependencies{
		Store:   mockStore,
		Metrics: newTestMetrics(),
	}

	reqBody := `{"jsonrpc":"2.0","id":"pt-configured","method":"resources/list","params":{}}`
	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		bytes.NewReader([]byte(reqBody)),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")

	err := deps.handleMCP(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp mcp.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Nil(t, resp.Error)
	require.NotNil(t, resp.Result)

	var result map[string]string
	require.NoError(t, json.Unmarshal(resp.Result, &result))
	require.Equal(t, "selected", result["source"])

	mockStore.AssertExpectations(t)
}

func TestHandleMCP_PassThrough_UpstreamFailure(t *testing.T) {
	// Create mock upstream that always fails
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, err := w.Write([]byte("internal server error"))
		require.NoError(t, err)
	}))
	defer upstreamServer.Close()

	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
	mockStore.On("GetTenantConfig", mock.Anything, "t_demo").
		Return(models.TenantConfig{}, store.ErrNotFound)
	mockStore.On("ListTools", mock.Anything, "t_demo", false, mock.Anything).Return([]models.Tool{
		{
			ToolID:         "mcp_tool",
			TenantID:       "t_demo",
			Transport:      "mcp_streamable_http",
			MCPUpstreamURL: upstreamServer.URL,
		},
	}, nil)

	deps := &Dependencies{
		Store:   mockStore,
		Metrics: newTestMetrics(),
	}

	reqBody := `{"jsonrpc":"2.0","id":"pt-2","method":"resources/list","params":{}}`

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		bytes.NewReader([]byte(reqBody)),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")

	err := deps.handleMCP(ctx)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp mcp.Response
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	// Upstream HTTP 500 is handled by MCPClient as a JSON-RPC error response
	// (not a Go error), so it's returned as an upstream error response
	assert.NotNil(t, resp.Error)
	assert.Equal(t, mcp.ErrorInternalError, resp.Error.Code)

	mockStore.AssertExpectations(t)
}

func TestHandleMCP_PassThrough_NotificationNoResponse(t *testing.T) {
	// Create mock upstream MCP server
	var upstreamCalled atomic.Bool
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamServer.Close()

	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
	mockStore.On("GetTenantConfig", mock.Anything, "t_demo").
		Return(models.TenantConfig{}, store.ErrNotFound)
	mockStore.On("ListTools", mock.Anything, "t_demo", false, mock.Anything).Return([]models.Tool{
		{
			ToolID:         "mcp_tool",
			TenantID:       "t_demo",
			Transport:      "mcp_streamable_http",
			MCPUpstreamURL: upstreamServer.URL,
		},
	}, nil)

	deps := &Dependencies{
		Store:   mockStore,
		Metrics: newTestMetrics(),
	}

	// Notification: no "id" field
	reqBody := `{"jsonrpc":"2.0","method":"notifications/custom","params":{}}`

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		bytes.NewReader([]byte(reqBody)),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")

	err := deps.handleMCP(ctx)
	require.NoError(t, err)
	// Notifications return 202 with no body
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.String())

	// Verify upstream was actually called (pass-through forwards the notification)
	// even though handleMCP suppresses the response for notifications
	assert.True(t, upstreamCalled.Load(), "upstream should have been called for notification pass-through")

	mockStore.AssertExpectations(t)
}

func TestHandleMCP_PassThrough_SkipsNonMCPTools(t *testing.T) {
	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
	mockStore.On("GetTenantConfig", mock.Anything, "t_demo").
		Return(models.TenantConfig{}, store.ErrNotFound)
	// Only non-MCP tools available - should fall back to method not found
	mockStore.On("ListTools", mock.Anything, "t_demo", false, mock.Anything).Return([]models.Tool{
		{
			ToolID:    "rest_tool",
			TenantID:  "t_demo",
			Transport: "http",
			BaseURL:   "http://localhost:8090",
		},
	}, nil)

	deps := &Dependencies{
		Store:   mockStore,
		Metrics: newTestMetrics(),
	}

	reqBody := `{"jsonrpc":"2.0","id":"pt-3","method":"prompts/list","params":{}}`

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		bytes.NewReader([]byte(reqBody)),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")

	err := deps.handleMCP(ctx)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp mcp.Response
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotNil(t, resp.Error)
	assert.Equal(t, mcp.ErrorMethodNotFound, resp.Error.Code)

	mockStore.AssertExpectations(t)
}

func TestHandleMCP_PassThrough_InvalidConfiguredToolReturnsInternalError(t *testing.T) {
	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
	mockStore.On("GetTenantConfig", mock.Anything, "t_demo").
		Return(models.TenantConfig{
			TenantID:                     "t_demo",
			MCPPassthroughUpstreamToolID: "missing_tool",
		}, nil)
	mockStore.On("ListTools", mock.Anything, "t_demo", false, mock.Anything).Return([]models.Tool{
		{
			ToolID:         "mcp_tool",
			TenantID:       "t_demo",
			Transport:      "mcp_streamable_http",
			MCPUpstreamURL: "http://example.com",
		},
	}, nil)

	deps := &Dependencies{
		Store:   mockStore,
		Metrics: newTestMetrics(),
	}

	reqBody := `{"jsonrpc":"2.0","id":"pt-invalid","method":"resources/list","params":{}}`
	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		bytes.NewReader([]byte(reqBody)),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")

	err := deps.handleMCP(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp mcp.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.Equal(t, mcp.ErrorInternalError, resp.Error.Code)

	mockStore.AssertExpectations(t)
}

func TestHandleMCP_ToolsCallRateLimitExceeded(t *testing.T) {
	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
	mockStore.On("GetTool", mock.Anything, "t_demo", "mock_internal").
		Return(models.Tool{
			ToolID:         "mock_internal",
			TenantID:       "t_demo",
			Transport:      "mcp_streamable_http",
			MCPUpstreamURL: "http://example.test",
		}, nil)
	mockStore.On("GetRiskOverride", mock.Anything, "t_demo", "MCP.mock_internal").
		Return("", store.ErrNotFound)
	mockStore.On("GetEffectiveRateLimitConfig", mock.Anything, "t_demo").
		Return(models.RateLimitConfig{
			PerMinute: 1,
			PerDay:    10000,
			Scope:     "tenant_agent_tool",
		}, nil)
	mockStore.On(
		"IncrementRateLimitCounter",
		mock.Anything,
		"t_demo",
		"agent1",
		"mock_internal",
		"",
		"minute",
		mock.Anything,
		mock.Anything,
		int64(1),
	).Return(false, int64(0), nil)
	mockStore.On("InsertADR", mock.Anything, mock.MatchedBy(func(record *models.ActionDecisionRecord) bool {
		return record != nil &&
			record.Decision == string(decisionDeny) &&
			record.RuleID == "rate_limit_minute" &&
			len(record.Reasons) == 1 &&
			record.Reasons[0].Code == "RATE_LIMIT_EXCEEDED"
	})).Return(nil)

	policyMock := policy.NewMockEvaluatorAPI(t)
	policyMock.On("Evaluate", mock.Anything, "t_demo", mock.Anything).
		Return(policy.Result{
			Version:       "2026-01-20",
			Decision:      "ALLOW",
			Risk:          "MEDIUM",
			Rule:          models.DecisionRule{ID: "rule_allow", Priority: 10},
			Reasons:       []models.DecisionReason{{Code: "ALLOW", Message: "ok"}},
			Constraints:   map[string]any{},
			PolicyVersion: "p_v1",
		}, nil)

	deps := &Dependencies{
		Store:   mockStore,
		Policy:  policyMock,
		Metrics: newTestMetrics(),
		Config: config.Config{
			FeatureRateLimiting: true,
		},
	}

	reqBody := `{"jsonrpc":"2.0","id":"rl-1","method":"tools/call","params":{"name":"mock_internal","arguments":{"foo":"bar"}}}`
	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		bytes.NewReader([]byte(reqBody)),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")

	err := deps.handleMCP(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp mcp.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.Equal(t, mcp.ErrorRateLimitExceeded, resp.Error.Code)
}

func TestHandleMCP_ToolsCallArgConstraintDenied(t *testing.T) {
	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
	mockStore.On("GetTool", mock.Anything, "t_demo", "mock_internal").
		Return(models.Tool{
			ToolID:         "mock_internal",
			TenantID:       "t_demo",
			Transport:      "mcp_streamable_http",
			MCPUpstreamURL: "http://example.test",
		}, nil)
	mockStore.On("GetRiskOverride", mock.Anything, "t_demo", "MCP.mock_internal").
		Return("", store.ErrNotFound)
	mockStore.On("InsertADR", mock.Anything, mock.MatchedBy(func(record *models.ActionDecisionRecord) bool {
		if record == nil {
			return false
		}
		failures, ok := record.Constraints["arg_constraint_failures"].([]map[string]any)
		return record.Decision == string(decisionDeny) &&
			record.RuleID == "deny_prod_branch" &&
			len(record.Reasons) == 1 &&
			record.Reasons[0].Code == argConstraintReasonDeny &&
			ok &&
			len(failures) == 1
	})).Return(nil)

	policyMock := policy.NewMockEvaluatorAPI(t)
	policyMock.On("Evaluate", mock.Anything, "t_demo", mock.Anything).
		Return(policy.Result{
			Version:  "2026-01-20",
			Decision: "ALLOW",
			Risk:     "MEDIUM",
			Rule:     models.DecisionRule{ID: "rule_allow", Priority: 10},
			Reasons:  []models.DecisionReason{{Code: "ALLOW", Message: "ok"}},
			Constraints: map[string]any{
				"args": map[string]any{
					"deny": []any{
						map[string]any{
							"id":      "deny_prod_branch",
							"path":    "/branch",
							"op":      "prefix",
							"value":   "prod/",
							"message": "branch blocked",
						},
					},
				},
			},
			PolicyVersion: "p_v1",
		}, nil)

	deps := &Dependencies{
		Store:   mockStore,
		Policy:  policyMock,
		Metrics: newTestMetrics(),
		Config: config.Config{
			FeatureArgConstraints: true,
		},
	}

	reqBody := `{"jsonrpc":"2.0","id":"ac-1","method":"tools/call","params":{"name":"mock_internal","arguments":{"branch":"prod/release"}}}`
	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		bytes.NewReader([]byte(reqBody)),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")

	err := deps.handleMCP(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp mcp.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.Equal(t, mcp.ErrorDeniedByPolicy, resp.Error.Code)
}

func TestHandleMCP_ToolsCallPolicyInputStripsInternalArgs(t *testing.T) {
	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
	mockStore.On("GetTool", mock.Anything, "t_demo", "mock_internal").
		Return(models.Tool{
			ToolID:         "mock_internal",
			TenantID:       "t_demo",
			Transport:      "mcp_streamable_http",
			MCPUpstreamURL: "http://example.test",
		}, nil)
	mockStore.On("GetRiskOverride", mock.Anything, "t_demo", "MCP.mock_internal").
		Return("", store.ErrNotFound)
	mockStore.On("InsertADR", mock.Anything, mock.Anything).Return(nil)

	policyMock := policy.NewMockEvaluatorAPI(t)
	policyMock.
		On("Evaluate", mock.Anything, "t_demo", mock.Anything).
		Run(func(args mock.Arguments) {
			input, ok := args.Get(2).(map[string]any)
			require.True(t, ok)
			arguments, ok := input["arguments"].(map[string]any)
			require.True(t, ok)
			_, hasControlField := arguments["_rbitr_approval_request_id"]
			require.False(t, hasControlField)
			require.Equal(t, "main", arguments["branch"])
		}).
		Return(policy.Result{
			Version:       "2026-01-20",
			Decision:      "DENY",
			Risk:          "MEDIUM",
			Rule:          models.DecisionRule{ID: "rule_deny", Priority: 100},
			Reasons:       []models.DecisionReason{{Code: "DENY", Message: "blocked"}},
			Constraints:   map[string]any{},
			PolicyVersion: "p_v1",
		}, nil)

	deps := &Dependencies{
		Store:   mockStore,
		Policy:  policyMock,
		Metrics: newTestMetrics(),
	}

	reqBody := `{"jsonrpc":"2.0","id":"ac-2","method":"tools/call","params":{"name":"mock_internal","arguments":{"branch":"main","_rbitr_approval_request_id":"ar_old"}}}`
	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		bytes.NewReader([]byte(reqBody)),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")

	err := deps.handleMCP(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp mcp.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.Equal(t, mcp.ErrorDeniedByPolicy, resp.Error.Code)
}

func TestHandleMCP_ToolsCallDeniedIncludesMatchedRules(t *testing.T) {
	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
	mockStore.On("GetTool", mock.Anything, "t_demo", "mock_internal").
		Return(models.Tool{
			ToolID:         "mock_internal",
			TenantID:       "t_demo",
			Transport:      "mcp_streamable_http",
			MCPUpstreamURL: "http://example.test",
		}, nil)
	mockStore.On("GetRiskOverride", mock.Anything, "t_demo", "MCP.mock_internal").
		Return("", store.ErrNotFound)
	mockStore.On("InsertADR", mock.Anything, mock.Anything).Return(nil)

	policyMock := policy.NewMockEvaluatorAPI(t)
	policyMock.
		On("Evaluate", mock.Anything, "t_demo", mock.Anything).
		Return(policy.Result{
			Version:     "2026-01-20",
			Decision:    "DENY",
			Risk:        "MEDIUM",
			Rule:        models.DecisionRule{ID: "rule_deny_high", Priority: 100},
			Reasons:     []models.DecisionReason{{Code: "DENY", Message: "blocked"}},
			Constraints: map[string]any{},
			MatchedRules: []models.DecisionMatchedRule{
				{RuleID: "rule_deny_high", Priority: 100, Effect: "DENY"},
				{RuleID: "rule_allow_low", Priority: 10, Effect: "ALLOW"},
			},
			PolicyVersion: "p_v1",
		}, nil)

	deps := &Dependencies{
		Store:   mockStore,
		Policy:  policyMock,
		Metrics: newTestMetrics(),
	}

	reqBody := `{"jsonrpc":"2.0","id":"mr-1","method":"tools/call","params":{"name":"mock_internal","arguments":{"branch":"main"}}}`
	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		bytes.NewReader([]byte(reqBody)),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")

	err := deps.handleMCP(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp mcp.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.Equal(t, mcp.ErrorDeniedByPolicy, resp.Error.Code)
	var data map[string]any
	require.NoError(t, json.Unmarshal(resp.Error.Data, &data))
	matchedRules, ok := data["matched_rules"].([]any)
	require.True(t, ok)
	require.Len(t, matchedRules, 2)
}

func TestHandleMCP_ToolsCallDenyShadowModeExecutes(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request mcp.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		result, _ := json.Marshal(map[string]any{
			"status": "ok",
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(mcp.Response{
			JSONRPC: "2.0",
			ID:      request.ID,
			Result:  result,
		})
	}))
	defer upstreamServer.Close()

	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
	mockStore.On("GetTool", mock.Anything, "t_demo", "mock_internal").
		Return(models.Tool{
			ToolID:         "mock_internal",
			TenantID:       "t_demo",
			Transport:      "mcp_streamable_http",
			MCPUpstreamURL: upstreamServer.URL,
		}, nil)
	mockStore.On("GetTenantConfig", mock.Anything, "t_demo").Return(models.TenantConfig{
		TenantID:            "t_demo",
		ActivePolicyVersion: "p_v1",
		EnforcementMode:     enforcementModeShadow,
	}, nil)
	mockStore.On("GetRiskOverride", mock.Anything, "t_demo", "MCP.mock_internal").
		Return("", store.ErrNotFound)
	mockStore.On("InsertADR", mock.Anything, mock.MatchedBy(func(record *models.ActionDecisionRecord) bool {
		return record != nil && record.Decision == string(decisionDeny) && record.RuleID == "rule_deny"
	})).Return(nil)

	policyMock := policy.NewMockEvaluatorAPI(t)
	policyMock.
		On("Evaluate", mock.Anything, "t_demo", mock.Anything).
		Return(policy.Result{
			Version:       "2026-01-20",
			Decision:      string(decisionDeny),
			Risk:          "MEDIUM",
			Rule:          models.DecisionRule{ID: "rule_deny", Priority: 100},
			Reasons:       []models.DecisionReason{{Code: "DENY", Message: "blocked"}},
			Constraints:   map[string]any{},
			PolicyVersion: "p_v1",
		}, nil)

	deps := &Dependencies{
		Store:   mockStore,
		Policy:  policyMock,
		Metrics: newTestMetrics(),
		Config: config.Config{
			FeatureShadowMode: true,
		},
	}

	reqBody := `{"jsonrpc":"2.0","id":"shadow-1","method":"tools/call","params":{"name":"mock_internal","arguments":{"branch":"main"}}}`
	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		bytes.NewReader([]byte(reqBody)),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t_demo"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")

	err := deps.handleMCP(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	var response mcp.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Nil(t, response.Error)

	var result map[string]any
	require.NoError(t, json.Unmarshal(response.Result, &result))
	require.Equal(t, "ok", result["status"])

	shadowRaw, ok := result["_rbitr_shadow"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, string(decisionDeny), shadowRaw["original_decision"])
	require.Equal(t, "rule_deny", shadowRaw["rule_id"])
}
