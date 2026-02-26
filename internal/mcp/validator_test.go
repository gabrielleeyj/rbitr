package mcp

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAndParseRequest(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectErr bool
		errCode   int
		validate  func(*testing.T, *Request)
	}{
		{
			name: "valid request with string id",
			input: `{
				"jsonrpc": "2.0",
				"id": "req-1",
				"method": "tools/list",
				"params": {}
			}`,
			validate: func(t *testing.T, req *Request) {
				assert.Equal(t, "2.0", req.JSONRPC)
				assert.Equal(t, "tools/list", req.Method)
				require.NotNil(t, req.ID)
				assert.NotNil(t, req.ID.String())
				assert.Equal(t, "req-1", *req.ID.String())
			},
		},
		{
			name: "valid request with number id",
			input: `{
				"jsonrpc": "2.0",
				"id": 123,
				"method": "tools/call",
				"params": {"name": "jira"}
			}`,
			validate: func(t *testing.T, req *Request) {
				assert.Equal(t, "2.0", req.JSONRPC)
				assert.Equal(t, "tools/call", req.Method)
				require.NotNil(t, req.ID)
				assert.NotNil(t, req.ID.Number())
				assert.Equal(t, float64(123), *req.ID.Number())
			},
		},
		{
			name: "null id is invalid (notifications must omit id)",
			input: `{
				"jsonrpc": "2.0",
				"id": null,
				"method": "tools/list"
			}`,
			expectErr: true,
			errCode:   ErrorInvalidRequest,
		},
		{
			name: "valid request without params",
			input: `{
				"jsonrpc": "2.0",
				"id": 1,
				"method": "tools/list"
			}`,
			validate: func(t *testing.T, req *Request) {
				assert.Equal(t, "2.0", req.JSONRPC)
				assert.Equal(t, "tools/list", req.Method)
			},
		},
		{
			name:      "invalid json",
			input:     `{invalid json`,
			expectErr: true,
			errCode:   ErrorParseError,
		},
		{
			name: "missing jsonrpc field",
			input: `{
				"id": 1,
				"method": "tools/list"
			}`,
			expectErr: true,
			errCode:   ErrorInvalidRequest,
		},
		{
			name: "wrong jsonrpc version",
			input: `{
				"jsonrpc": "1.0",
				"id": 1,
				"method": "tools/list"
			}`,
			expectErr: true,
			errCode:   ErrorInvalidRequest,
		},
		{
			name: "missing method",
			input: `{
				"jsonrpc": "2.0",
				"id": 1
			}`,
			expectErr: true,
			errCode:   ErrorInvalidRequest,
		},
		{
			name:      "empty method",
			input:     `{"jsonrpc": "2.0", "id": 1, "method": ""}`,
			expectErr: true,
			errCode:   ErrorInvalidRequest,
		},
		{
			name:      "trailing data",
			input:     `{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}{"extra": "json"}`,
			expectErr: true,
			errCode:   ErrorInvalidRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := ValidateAndParseRequest([]byte(tt.input), MaxRequestSize)

			if tt.expectErr {
				require.Error(t, err)
				errObj := &ErrorObject{}
				ok := errors.As(err, &errObj)
				require.True(t, ok, "error should be an ErrorObject")
				assert.Equal(t, tt.errCode, errObj.Code)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, req)
			if tt.validate != nil {
				tt.validate(t, req)
			}
		})
	}
}

func TestValidateAndParseRequest_SizeLimit(t *testing.T) {
	// Create a request that exceeds the size limit
	largeParams := strings.Repeat("x", 1000)
	input := `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "tools/call",
		"params": {"data": "` + largeParams + `"}
	}`

	// Should succeed with normal limit
	req, err := ValidateAndParseRequest([]byte(input), MaxRequestSize)
	require.NoError(t, err)
	require.NotNil(t, req)

	// Should fail with very small limit
	_, err = ValidateAndParseRequest([]byte(input), 100)
	require.Error(t, err)
	errObj := &ErrorObject{}
	ok := errors.As(err, &errObj)
	require.True(t, ok)
	assert.Equal(t, ErrorInvalidRequest, errObj.Code)
	assert.Contains(t, errObj.Message, "too large")
}

func TestValidateAndParseRequest_ExactBoundary(t *testing.T) {
	// Test the exact boundary case: valid JSON at maxSize with extra bytes after
	validJSON := `{"jsonrpc":"2.0","id":1,"method":"test"}`
	extraBytes := "extra data after json"
	input := validJSON + extraBytes

	// Set limit to exact size of valid JSON
	maxSize := int64(len(validJSON))

	// Should fail because total input exceeds maxSize
	_, err := ValidateAndParseRequest([]byte(input), maxSize)
	require.Error(t, err)
	errObj := &ErrorObject{}
	ok := errors.As(err, &errObj)
	require.True(t, ok)
	assert.Equal(t, ErrorInvalidRequest, errObj.Code)
	assert.Contains(t, errObj.Message, "too large")

	// Should succeed with input exactly at maxSize
	_, err = ValidateAndParseRequest([]byte(validJSON), maxSize)
	require.NoError(t, err)

	// Should succeed with input under maxSize
	_, err = ValidateAndParseRequest([]byte(validJSON), maxSize+100)
	require.NoError(t, err)
}

func TestNewSuccessResponse(t *testing.T) {
	result := ToolsListResult{
		Tools: []Tool{
			{Name: "jira", Description: "Jira", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
	}
	id := NewNumberID(1)

	resp, err := NewSuccessResponse(id, result)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "2.0", resp.JSONRPC)
	assert.Equal(t, id, resp.ID)
	assert.Nil(t, resp.Error)
	assert.NotNil(t, resp.Result)

	// Verify result can be unmarshaled
	var decodedResult ToolsListResult
	err = json.Unmarshal(resp.Result, &decodedResult)
	require.NoError(t, err)
	assert.Len(t, decodedResult.Tools, 1)
}

func TestNewErrorResponse(t *testing.T) {
	id := NewStringID("req-1")
	errObj := &ErrorObject{
		Code:    ErrorMethodNotFound,
		Message: "method not found",
	}

	resp := NewErrorResponse(id, errObj)
	require.NotNil(t, resp)

	assert.Equal(t, "2.0", resp.JSONRPC)
	assert.Equal(t, id, resp.ID)
	assert.Nil(t, resp.Result)
	assert.Equal(t, errObj, resp.Error)
}

func TestErrorConstructors(t *testing.T) {
	t.Run("NewMethodNotFoundError", func(t *testing.T) {
		err := NewMethodNotFoundError("tools/foo")
		assert.Equal(t, ErrorMethodNotFound, err.Code)
		assert.Contains(t, err.Message, "tools/foo")
	})

	t.Run("NewInvalidParamsError", func(t *testing.T) {
		err := NewInvalidParamsError("missing name field")
		assert.Equal(t, ErrorInvalidParams, err.Code)
		assert.Contains(t, err.Message, "missing name field")
	})

	t.Run("NewInternalError", func(t *testing.T) {
		err := NewInternalError("database connection failed")
		assert.Equal(t, ErrorInternalError, err.Code)
		assert.Equal(t, "database connection failed", err.Message)
	})

	t.Run("NewApprovalRequiredError", func(t *testing.T) {
		data := &ApprovalRequiredData{
			ApprovalRequired:  true,
			ApprovalRequestID: "apr_123",
			ExpiresAt:         "2026-02-13T10:00:00Z",
			PolicyVersion:     "p_v1",
			RuleID:            "test_rule",
			Risk:              "HIGH",
			Reasons:           []string{"high risk action"},
		}
		err := NewApprovalRequiredError(data)
		assert.Equal(t, ErrorApprovalRequired, err.Code)
		assert.Equal(t, "approval required", err.Message)
		assert.NotNil(t, err.Data)

		var decoded ApprovalRequiredData
		jsonErr := json.Unmarshal(err.Data, &decoded)
		require.NoError(t, jsonErr)
		assert.Equal(t, "apr_123", decoded.ApprovalRequestID)
	})

	t.Run("NewDeniedError", func(t *testing.T) {
		data := &DeniedData{
			Denied:        true,
			PolicyVersion: "p_v1",
			RuleID:        "deny_rule",
			Risk:          "HIGH",
			Reasons:       []string{"action not allowed"},
			Tags:          []string{"policy", "deny"},
		}
		err := NewDeniedError(data)
		assert.Equal(t, ErrorDeniedByPolicy, err.Code)
		assert.Equal(t, "denied by policy", err.Message)
		assert.NotNil(t, err.Data)

		var decoded DeniedData
		jsonErr := json.Unmarshal(err.Data, &decoded)
		require.NoError(t, jsonErr)
		assert.Equal(t, "deny_rule", decoded.RuleID)
	})

	t.Run("NewPolicyInvalidError", func(t *testing.T) {
		err := NewPolicyInvalidError(PolicyErrorInvalidOutput)
		assert.Equal(t, ErrorPolicyInvalid, err.Code)
		assert.Equal(t, "policy evaluation error", err.Message)
		assert.NotNil(t, err.Data)

		var data map[string]string
		jsonErr := json.Unmarshal(err.Data, &data)
		require.NoError(t, jsonErr)
		assert.Equal(t, "POLICY_INVALID_OUTPUT", data["reason_code"])
	})
}
