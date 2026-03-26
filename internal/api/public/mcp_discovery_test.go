package public

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/mcp"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/testhelpers"
)

func TestEnrichToolSchema(t *testing.T) {
	t.Parallel()

	t.Run("skips openapi_import tools", func(t *testing.T) {
		tool := models.Tool{
			ToolID:    "stripe",
			Source:    "openapi_import",
			Transport: "http",
			BaseURL:   "https://api.stripe.com",
		}
		schema := json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)
		result := enrichToolSchema(&tool, schema)
		assert.Equal(t, string(schema), string(result))
	})

	t.Run("skips non-HTTP tools", func(t *testing.T) {
		tool := models.Tool{
			ToolID:    "mcp_tool",
			Source:    "admin",
			Transport: "mcp_streamable_http",
		}
		schema := json.RawMessage(`{"type":"object"}`)
		result := enrichToolSchema(&tool, schema)
		assert.Equal(t, string(schema), string(result))
	})

	t.Run("enriches manual HTTP tool", func(t *testing.T) {
		tool := models.Tool{
			ToolID:    "payment_api",
			Source:    "admin",
			Transport: "http",
			BaseURL:   "https://api.example.com",
		}
		schema := json.RawMessage(`{"type":"object","additionalProperties":true}`)
		result := enrichToolSchema(&tool, schema)

		var m map[string]any
		require.NoError(t, json.Unmarshal(result, &m))
		require.Contains(t, m, "x-rbitr-endpoints")

		endpoints, ok := m["x-rbitr-endpoints"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "https://api.example.com", endpoints["base_url"])
	})

	t.Run("enriches http_api transport", func(t *testing.T) {
		tool := models.Tool{
			ToolID:    "internal_api",
			Source:    "admin",
			Transport: "http_api",
			BaseURL:   "http://internal:8080",
		}
		schema := json.RawMessage(`{"type":"object"}`)
		result := enrichToolSchema(&tool, schema)

		var m map[string]any
		require.NoError(t, json.Unmarshal(result, &m))
		assert.Contains(t, m, "x-rbitr-endpoints")
	})

	t.Run("does not overwrite existing x-rbitr-endpoints", func(t *testing.T) {
		tool := models.Tool{
			ToolID:    "custom",
			Source:    "admin",
			Transport: "http",
			BaseURL:   "https://api.example.com",
		}
		schema := json.RawMessage(`{"type":"object","x-rbitr-endpoints":{"custom":"data"}}`)
		result := enrichToolSchema(&tool, schema)

		var m map[string]any
		require.NoError(t, json.Unmarshal(result, &m))
		endpoints, ok := m["x-rbitr-endpoints"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "data", endpoints["custom"])
		assert.NotContains(t, endpoints, "base_url")
	})
}

func TestParseResourceURI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		uri    string
		toolID string
		ok     bool
	}{
		{"valid", "rbitr://tools/stripe/openapi", "stripe", true},
		{"valid with hyphens", "rbitr://tools/payment-api/openapi", "payment-api", true},
		{"missing prefix", "other://tools/stripe/openapi", "", false},
		{"missing suffix", "rbitr://tools/stripe/schema", "", false},
		{"empty tool id", "rbitr://tools//openapi", "", false},
		{"empty string", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			toolID, ok := parseResourceURI(tc.uri)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.toolID, toolID)
		})
	}
}

func TestBuildResourceURI(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "rbitr://tools/stripe/openapi", buildResourceURI("stripe"))
	assert.Equal(t, "rbitr://tools/my-api/openapi", buildResourceURI("my-api"))
}

// sendMCPRequest is a test helper that sends an MCP request through handleMCP.
//
//nolint:unparam // tenantID kept as parameter for test flexibility
func sendMCPRequest(t *testing.T, deps *Dependencies, tenantID, method string, params any) mcp.Response {
	t.Helper()

	var paramsJSON json.RawMessage
	if params != nil {
		var err error
		paramsJSON, err = json.Marshal(params)
		require.NoError(t, err)
	} else {
		paramsJSON = json.RawMessage(`{}`)
	}

	reqObj := map[string]any{
		"jsonrpc": "2.0",
		"id":      "test-1",
		"method":  method,
		"params":  paramsJSON,
	}
	body, err := json.Marshal(reqObj)
	require.NoError(t, err)

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		bytes.NewReader(body),
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{tenantID}},
	)
	req.Header.Set(auth.TenantKeyHeader, "valid_key")
	req.Header.Set(auth.AgentIDHeader, "agent1")

	err = deps.handleMCP(ctx)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp mcp.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

func TestHandleMCP_ResourcesList(t *testing.T) {
	t.Run("returns resources for tools with OpenAPI specs", func(t *testing.T) {
		mockStore := newPublicStoreMock(t)
		mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
			Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
		mockStore.On("ListTools", mock.Anything, "t_demo", false, mock.Anything).Return([]models.Tool{
			{
				ToolID:         "stripe",
				TenantID:       "t_demo",
				Transport:      "http",
				BaseURL:        "https://api.stripe.com",
				OpenAPISpecURL: "https://api.stripe.com/openapi.json",
				Source:         "openapi_import",
			},
			{
				ToolID:    "manual_tool",
				TenantID:  "t_demo",
				Transport: "http",
				BaseURL:   "https://api.example.com",
				Source:    "admin",
				// No OpenAPI spec URL
			},
			{
				ToolID:         "jira",
				TenantID:       "t_demo",
				Transport:      "http",
				BaseURL:        "https://jira.example.com",
				OpenAPISpecURL: "https://jira.example.com/openapi.json",
				Source:         "openapi_import",
			},
		}, nil)

		deps := &Dependencies{
			Store:   mockStore,
			Metrics: newTestMetrics(),
		}

		resp := sendMCPRequest(t, deps, "t_demo", "resources/list", nil)
		require.Nil(t, resp.Error)
		require.NotNil(t, resp.Result)

		var result mcp.ResourcesListResult
		require.NoError(t, json.Unmarshal(resp.Result, &result))
		require.Len(t, result.Resources, 2)

		assert.Equal(t, "rbitr://tools/stripe/openapi", result.Resources[0].URI)
		assert.Equal(t, "stripe OpenAPI spec", result.Resources[0].Name)
		assert.Equal(t, "application/json", result.Resources[0].MIMEType)

		assert.Equal(t, "rbitr://tools/jira/openapi", result.Resources[1].URI)

		mockStore.AssertExpectations(t)
	})

	t.Run("returns empty list when no tools have specs", func(t *testing.T) {
		mockStore := newPublicStoreMock(t)
		mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
			Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
		mockStore.On("ListTools", mock.Anything, "t_demo", false, mock.Anything).Return([]models.Tool{
			{
				ToolID:    "manual_tool",
				TenantID:  "t_demo",
				Transport: "http",
				Source:    "admin",
			},
		}, nil)

		deps := &Dependencies{
			Store:   mockStore,
			Metrics: newTestMetrics(),
		}

		resp := sendMCPRequest(t, deps, "t_demo", "resources/list", nil)
		require.Nil(t, resp.Error)

		var result mcp.ResourcesListResult
		require.NoError(t, json.Unmarshal(resp.Result, &result))
		assert.Empty(t, result.Resources)
	})
}

func TestHandleMCP_ResourcesRead(t *testing.T) {
	t.Run("fetches OpenAPI spec for valid tool", func(t *testing.T) {
		specServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"openapi":"3.0.0","info":{"title":"Test API"}}`))
		}))
		defer specServer.Close()

		mockStore := newPublicStoreMock(t)
		mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
			Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
		mockStore.On("GetTool", mock.Anything, "t_demo", "stripe").
			Return(models.Tool{
				ToolID:         "stripe",
				TenantID:       "t_demo",
				OpenAPISpecURL: specServer.URL,
			}, nil)

		deps := &Dependencies{
			Store:   mockStore,
			Metrics: newTestMetrics(),
		}

		params := map[string]string{"uri": "rbitr://tools/stripe/openapi"}
		resp := sendMCPRequest(t, deps, "t_demo", "resources/read", params)
		require.Nil(t, resp.Error)

		var result mcp.ResourcesReadResult
		require.NoError(t, json.Unmarshal(resp.Result, &result))
		require.Len(t, result.Contents, 1)
		assert.Equal(t, "rbitr://tools/stripe/openapi", result.Contents[0].URI)
		assert.Equal(t, "application/json", result.Contents[0].MIMEType)
		assert.Contains(t, result.Contents[0].Text, `"openapi":"3.0.0"`)

		mockStore.AssertExpectations(t)
	})

	t.Run("returns error for missing URI", func(t *testing.T) {
		mockStore := newPublicStoreMock(t)
		mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
			Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)

		deps := &Dependencies{
			Store:   mockStore,
			Metrics: newTestMetrics(),
		}

		resp := sendMCPRequest(t, deps, "t_demo", "resources/read", map[string]string{})
		require.NotNil(t, resp.Error)
		assert.Equal(t, mcp.ErrorInvalidParams, resp.Error.Code)
		assert.Contains(t, resp.Error.Message, "uri is required")
	})

	t.Run("returns error for invalid URI format", func(t *testing.T) {
		mockStore := newPublicStoreMock(t)
		mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
			Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)

		deps := &Dependencies{
			Store:   mockStore,
			Metrics: newTestMetrics(),
		}

		params := map[string]string{"uri": "invalid://uri"}
		resp := sendMCPRequest(t, deps, "t_demo", "resources/read", params)
		require.NotNil(t, resp.Error)
		assert.Contains(t, resp.Error.Message, "invalid resource URI")
	})

	t.Run("returns error for nonexistent tool", func(t *testing.T) {
		mockStore := newPublicStoreMock(t)
		mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
			Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
		mockStore.On("GetTool", mock.Anything, "t_demo", "nonexistent").
			Return(models.Tool{}, store.ErrNotFound)

		deps := &Dependencies{
			Store:   mockStore,
			Metrics: newTestMetrics(),
		}

		params := map[string]string{"uri": "rbitr://tools/nonexistent/openapi"}
		resp := sendMCPRequest(t, deps, "t_demo", "resources/read", params)
		require.NotNil(t, resp.Error)
		assert.Contains(t, resp.Error.Message, "resource not found")
	})

	t.Run("returns error for tool without OpenAPI spec", func(t *testing.T) {
		mockStore := newPublicStoreMock(t)
		mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
			Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
		mockStore.On("GetTool", mock.Anything, "t_demo", "manual_tool").
			Return(models.Tool{
				ToolID:   "manual_tool",
				TenantID: "t_demo",
				Source:   "admin",
			}, nil)

		deps := &Dependencies{
			Store:   mockStore,
			Metrics: newTestMetrics(),
		}

		params := map[string]string{"uri": "rbitr://tools/manual_tool/openapi"}
		resp := sendMCPRequest(t, deps, "t_demo", "resources/read", params)
		require.NotNil(t, resp.Error)
		assert.Contains(t, resp.Error.Message, "no OpenAPI spec available")
	})
}

func TestHandleMCP_ToolsListEnrichment(t *testing.T) {
	t.Run("enriches manual HTTP tools with x-rbitr-endpoints", func(t *testing.T) {
		mockStore := newPublicStoreMock(t)
		mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
			Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
		mockStore.On("ListTools", mock.Anything, "t_demo", false, mock.Anything).Return([]models.Tool{
			{
				ToolID:          "manual_api",
				TenantID:        "t_demo",
				Transport:       "http",
				BaseURL:         "https://api.example.com",
				Source:          "admin",
				Description:     "Manual API",
				InputSchemaJSON: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
			},
		}, nil)

		deps := &Dependencies{
			Store:   mockStore,
			Metrics: newTestMetrics(),
		}

		resp := sendMCPRequest(t, deps, "t_demo", "tools/list", nil)
		require.Nil(t, resp.Error)

		var result mcp.ToolsListResult
		require.NoError(t, json.Unmarshal(resp.Result, &result))
		require.Len(t, result.Tools, 1)

		var schema map[string]any
		require.NoError(t, json.Unmarshal(result.Tools[0].InputSchema, &schema))
		require.Contains(t, schema, "x-rbitr-endpoints")

		endpoints, ok := schema["x-rbitr-endpoints"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "https://api.example.com", endpoints["base_url"])
	})

	t.Run("does not enrich openapi_import tools", func(t *testing.T) {
		mockStore := newPublicStoreMock(t)
		mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
			Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)
		mockStore.On("ListTools", mock.Anything, "t_demo", false, mock.Anything).Return([]models.Tool{
			{
				ToolID:          "stripe_charge",
				TenantID:        "t_demo",
				Transport:       "http",
				BaseURL:         "https://api.stripe.com",
				Source:          "openapi_import",
				Description:     "Create a charge",
				InputSchemaJSON: json.RawMessage(`{"type":"object","properties":{"amount":{"type":"integer"}}}`),
			},
		}, nil)

		deps := &Dependencies{
			Store:   mockStore,
			Metrics: newTestMetrics(),
		}

		resp := sendMCPRequest(t, deps, "t_demo", "tools/list", nil)
		require.Nil(t, resp.Error)

		var result mcp.ToolsListResult
		require.NoError(t, json.Unmarshal(resp.Result, &result))
		require.Len(t, result.Tools, 1)

		var schema map[string]any
		require.NoError(t, json.Unmarshal(result.Tools[0].InputSchema, &schema))
		assert.NotContains(t, schema, "x-rbitr-endpoints")
	})
}

func TestHandleMCP_InitializeAdvertisesResources(t *testing.T) {
	mockStore := newPublicStoreMock(t)
	mockStore.On("GetTenantByKeyHash", mock.Anything, mock.Anything).
		Return(models.Tenant{TenantID: "t_demo", Name: "Demo", Enabled: true}, nil)

	deps := &Dependencies{
		Store:   mockStore,
		Metrics: newTestMetrics(),
	}

	params := map[string]any{
		"protocolVersion": "2025-11-25",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": "test-client", "version": "1.0"},
	}
	resp := sendMCPRequest(t, deps, "t_demo", "initialize", params)
	require.Nil(t, resp.Error)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp.Result, &result))

	capabilities, ok := result["capabilities"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, capabilities, "tools")
	assert.Contains(t, capabilities, "resources")
}
