package public

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/mcp"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
	"github.com/gabrielleeyj/rbitr/internal/testhelpers"
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
					Return(models.Tenant{TenantID: "t_demo", Name: "Demo"}, nil)
			},
			expectedError:  true,
			expectedErrMsg: "tenant mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := &store.MockStoreAPI{}
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
	mockStore := &store.MockStoreAPI{}
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo"}, nil)
	mockStore.On("ListTools", mock.Anything, "t_demo").Return([]models.Tool{
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
			mockStore := &store.MockStoreAPI{}
			mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
				Return(models.Tenant{TenantID: "t_demo", Name: "Demo"}, nil)

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
		expectedErrCode int
	}{
		{
			name:            "tools/call not implemented",
			method:          "tools/call",
			expectedErrCode: mcp.ErrorMethodNotFound,
		},
		{
			name:            "unknown method",
			method:          "unknown/method",
			expectedErrCode: mcp.ErrorMethodNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := &store.MockStoreAPI{}
			mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
				Return(models.Tenant{TenantID: "t_demo", Name: "Demo"}, nil)

			deps := &Dependencies{
				Store:   mockStore,
				Metrics: newTestMetrics(),
			}

			reqBody := `{"jsonrpc":"2.0","id":1,"method":"` + tt.method + `","params":{}}`

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
					Return(models.Tenant{TenantID: "t_demo", Name: "Demo"}, nil)
				m.On("ListTools", mock.Anything, "t_demo").Return([]models.Tool{
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
					Return(models.Tenant{TenantID: "t_demo", Name: "Demo"}, nil)
				m.On("ListTools", mock.Anything, "t_demo").Return([]models.Tool{
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
					Return(models.Tenant{TenantID: "t_demo", Name: "Demo"}, nil)
				m.On("ListTools", mock.Anything, "t_demo").Return([]models.Tool{}, nil)
			},
			validateTools: func(t *testing.T, tools []mcp.Tool) {
				assert.Empty(t, tools)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := &store.MockStoreAPI{}
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
	mockStore := &store.MockStoreAPI{}
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo"}, nil)
	mockStore.On("ListTools", mock.Anything, "t_demo").
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
			name:      "null id (notification - no response)",
			requestID: `null`,
			checkIDFn: func(t *testing.T, id *mcp.RequestID) {
				// Should not get here - notifications don't get responses
				t.Fatal("notification should not receive a response")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := &store.MockStoreAPI{}
			mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
				Return(models.Tenant{TenantID: "t_demo", Name: "Demo"}, nil)
			mockStore.On("ListTools", mock.Anything, "t_demo").Return([]models.Tool{}, nil)

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

			// For notification (null ID), expect no response (204 No Content)
			if tt.requestID == `null` {
				assert.Equal(t, http.StatusNoContent, rec.Code)
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
