package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/testhelpers"
)

func TestHandleToolCreate(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		payload      CreateToolRequest
		config       config.Config
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		expectedErr  bool
	}{
		{
			name:         "no auth",
			expectedCode: http.StatusUnauthorized,
			expectedErr:  true,
		},
		{
			name:         "insufficient scope",
			adminKey:     "key",
			scopes:       []string{"admin:read"},
			expectedCode: http.StatusForbidden,
			expectedErr:  true,
		},
		{
			name:         "invalid tool_id — too short",
			adminKey:     "key",
			scopes:       []string{"admin:tools:write"},
			payload:      CreateToolRequest{ToolID: "ab"},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "invalid tool_id — starts with hyphen",
			adminKey:     "key",
			scopes:       []string{"admin:tools:write"},
			payload:      CreateToolRequest{ToolID: "-abc"},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "invalid transport",
			adminKey:     "key",
			scopes:       []string{"admin:tools:write"},
			payload:      CreateToolRequest{ToolID: "my_tool", Transport: "grpc"},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "invalid auth_type",
			adminKey:     "key",
			scopes:       []string{"admin:tools:write"},
			payload:      CreateToolRequest{ToolID: "my_tool", AuthType: "magic"},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "http transport missing base_url",
			adminKey:     "key",
			scopes:       []string{"admin:tools:write"},
			payload:      CreateToolRequest{ToolID: "my_tool", Transport: "http"},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "mcp transport missing mcp_upstream_url",
			adminKey:     "key",
			scopes:       []string{"admin:tools:write"},
			payload:      CreateToolRequest{ToolID: "my_tool", Transport: "mcp"},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "invalid input_schema_json",
			adminKey: "key",
			scopes:   []string{"admin:tools:write"},
			payload: CreateToolRequest{
				ToolID:          "my_tool",
				Transport:       "http",
				BaseURL:         "https://api.example.com",
				InputSchemaJSON: json.RawMessage(`"not an object"`),
			},
			config:       config.Config{OutboundAllowPrivate: true},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "duplicate tool_id",
			adminKey: "key",
			scopes:   []string{"admin:tools:write"},
			payload: CreateToolRequest{
				ToolID:    "my_tool",
				Transport: "http",
				BaseURL:   "https://api.example.com",
			},
			config: config.Config{OutboundAllowPrivate: true},
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("InsertTool", context.Background(), mock.Anything).Return(store.ErrDuplicate)
			},
			expectedCode: http.StatusConflict,
		},
		{
			name:     "success — http transport",
			adminKey: "key",
			scopes:   []string{"admin:tools:write"},
			payload: CreateToolRequest{
				ToolID:    "my_tool",
				Transport: "http",
				BaseURL:   "https://api.example.com",
				AuthType:  "bearer",
				AuthValue: "tok",
			},
			config: config.Config{OutboundAllowPrivate: true},
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("InsertTool", context.Background(), mock.Anything).Return(nil)
				m.On("InsertAuditEvent", context.Background(), mock.Anything).Return(nil)
			},
			expectedCode: http.StatusCreated,
		},
		{
			name:     "success — mcp transport",
			adminKey: "key",
			scopes:   []string{"admin:tools:write"},
			payload: CreateToolRequest{
				ToolID:         "mcp_tool",
				Transport:      "mcp",
				MCPUpstreamURL: "https://mcp.example.com/sse",
				Description:    "An MCP tool",
			},
			config: config.Config{OutboundAllowPrivate: true},
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("InsertTool", context.Background(), mock.Anything).Return(nil)
				m.On("InsertAuditEvent", context.Background(), mock.Anything).Return(nil)
			},
			expectedCode: http.StatusCreated,
		},
		{
			name:     "defaults — transport defaults to http, auth_type defaults to none",
			adminKey: "key",
			scopes:   []string{"admin:tools:write"},
			payload: CreateToolRequest{
				ToolID:  "default_tool",
				BaseURL: "https://api.example.com",
			},
			config: config.Config{OutboundAllowPrivate: true},
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("InsertTool", context.Background(), mock.MatchedBy(func(tool *models.Tool) bool {
					return tool.Transport == "http" && tool.AuthType == "none"
				})).Return(nil)
				m.On("InsertAuditEvent", context.Background(), mock.Anything).Return(nil)
			},
			expectedCode: http.StatusCreated,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storeMock := store.NewMockStoreAPI(t)
			if tc.adminKey != "" {
				storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
					Return(modelsAdminKey(tc.scopes), nil)
			}
			if tc.storeSetup != nil {
				tc.storeSetup(storeMock)
			}

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPost,
				testhelpers.MakeBody(tc.payload),
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: tc.config}
			err := deps.handleToolCreate(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleToolGet(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		expectedErr  bool
	}{
		{
			name:         "no auth",
			expectedCode: http.StatusUnauthorized,
			expectedErr:  true,
		},
		{
			name:     "not found",
			adminKey: "key",
			scopes:   []string{"admin:tools:read"},
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("GetTool", context.Background(), "t1", "my_tool").Return(models.Tool{}, store.ErrNotFound)
			},
			expectedCode: http.StatusNotFound,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:tools:read"},
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("GetTool", context.Background(), "t1", "my_tool").Return(models.Tool{
					ToolID:   "my_tool",
					TenantID: "t1",
					BaseURL:  "https://api.example.com",
					AuthType: "bearer",
				}, nil)
			},
			expectedCode: http.StatusOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storeMock := store.NewMockStoreAPI(t)
			if tc.adminKey != "" {
				storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
					Return(modelsAdminKey(tc.scopes), nil)
			}
			if tc.storeSetup != nil {
				tc.storeSetup(storeMock)
			}

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodGet,
				nil,
				testhelpers.Params{
					Names:  []string{"tenant_id", "tool_id"},
					Values: []string{"t1", "my_tool"},
				},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handleToolGet(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)

			if tc.name == "success" {
				var resp ToolResponse
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				require.Equal(t, "my_tool", resp.ToolID)
				require.NotNil(t, resp.HTTP)
				require.Equal(t, "https://api.example.com", resp.HTTP.BaseURL)
			}
		})
	}
}

func TestHandleToolArchive(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		expectedErr  bool
	}{
		{
			name:         "no auth",
			expectedCode: http.StatusUnauthorized,
			expectedErr:  true,
		},
		{
			name:     "not found",
			adminKey: "key",
			scopes:   []string{"admin:tools:write"},
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("ArchiveTool", context.Background(), "t1", "my_tool").Return(store.ErrNotFound)
			},
			expectedCode: http.StatusNotFound,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:tools:write"},
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("ArchiveTool", context.Background(), "t1", "my_tool").Return(nil)
				m.On("InsertAuditEvent", context.Background(), mock.Anything).Return(nil)
			},
			expectedCode: http.StatusNoContent,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storeMock := store.NewMockStoreAPI(t)
			if tc.adminKey != "" {
				storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
					Return(modelsAdminKey(tc.scopes), nil)
			}
			if tc.storeSetup != nil {
				tc.storeSetup(storeMock)
			}

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodDelete,
				nil,
				testhelpers.Params{
					Names:  []string{"tenant_id", "tool_id"},
					Values: []string{"t1", "my_tool"},
				},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handleToolArchive(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleToolRestore(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		expectedErr  bool
	}{
		{
			name:         "no auth",
			expectedCode: http.StatusUnauthorized,
			expectedErr:  true,
		},
		{
			name:     "not found or not archived",
			adminKey: "key",
			scopes:   []string{"admin:tools:write"},
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("RestoreTool", context.Background(), "t1", "my_tool").Return(store.ErrNotFound)
			},
			expectedCode: http.StatusNotFound,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:tools:write"},
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("RestoreTool", context.Background(), "t1", "my_tool").Return(nil)
				m.On("InsertAuditEvent", context.Background(), mock.Anything).Return(nil)
			},
			expectedCode: http.StatusNoContent,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storeMock := store.NewMockStoreAPI(t)
			if tc.adminKey != "" {
				storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
					Return(modelsAdminKey(tc.scopes), nil)
			}
			if tc.storeSetup != nil {
				tc.storeSetup(storeMock)
			}

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPost,
				nil,
				testhelpers.Params{
					Names:  []string{"tenant_id", "tool_id"},
					Values: []string{"t1", "my_tool"},
				},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handleToolRestore(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestToolToResponse(t *testing.T) {
	t.Run("http tool", func(t *testing.T) {
		tool := models.Tool{
			ToolID:    "http_tool",
			TenantID:  "t1",
			BaseURL:   "https://api.example.com",
			AuthType:  "bearer",
			AuthValue: "secret",
		}
		resp := toolToResponse(&tool)
		require.Equal(t, "http_tool", resp.ToolID)
		require.NotNil(t, resp.HTTP)
		require.Equal(t, "https://api.example.com", resp.HTTP.BaseURL)
		require.Equal(t, "bearer", resp.HTTP.AuthType)
		require.True(t, resp.HTTP.AuthSet)
		require.Nil(t, resp.MCP)
		require.Nil(t, resp.ArchivedAt)
	})

	t.Run("mcp tool", func(t *testing.T) {
		tool := models.Tool{
			ToolID:         "mcp_tool",
			TenantID:       "t1",
			MCPUpstreamURL: "https://mcp.example.com/sse",
			Description:    "An MCP tool",
		}
		resp := toolToResponse(&tool)
		require.Equal(t, "mcp_tool", resp.ToolID)
		require.Nil(t, resp.HTTP)
		require.NotNil(t, resp.MCP)
		require.Equal(t, "https://mcp.example.com/sse", resp.MCP.UpstreamURL)
		require.Equal(t, "An MCP tool", resp.MCP.Description)
	})

	t.Run("no auth value — auth_set false", func(t *testing.T) {
		tool := models.Tool{
			ToolID:   "tool",
			TenantID: "t1",
			BaseURL:  "https://api.example.com",
			AuthType: "none",
		}
		resp := toolToResponse(&tool)
		require.NotNil(t, resp.HTTP)
		require.False(t, resp.HTTP.AuthSet)
	})
}
