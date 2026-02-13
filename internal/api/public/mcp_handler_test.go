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
	// Method not found is expected since we haven't implemented handlers yet
	assert.NotNil(t, resp.Error)
	assert.Equal(t, mcp.ErrorMethodNotFound, resp.Error.Code)

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
			name:            "tools/list not implemented",
			method:          "tools/list",
			expectedErrCode: mcp.ErrorMethodNotFound,
		},
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
