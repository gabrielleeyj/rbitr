package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestID_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		id       *RequestID
		expected string
	}{
		{
			name:     "null",
			id:       NewNullID(),
			expected: "null",
		},
		{
			name:     "string",
			id:       NewStringID("test-id"),
			expected: `"test-id"`,
		},
		{
			name:     "number",
			id:       NewNumberID(123),
			expected: "123",
		},
		{
			name:     "number decimal",
			id:       NewNumberID(123.45),
			expected: "123.45",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.id)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, string(data))
		})
	}
}

func TestRequestID_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		expectNull bool
		expectStr  *string
		expectNum  *float64
		expectErr  bool
	}{
		{
			name:      "null is invalid for request IDs",
			input:     "null",
			expectErr: true,
		},
		{
			name:      "string",
			input:     `"test-id"`,
			expectStr: stringPtr("test-id"),
		},
		{
			name:      "number integer",
			input:     "123",
			expectNum: float64Ptr(123),
		},
		{
			name:      "number decimal",
			input:     "123.45",
			expectNum: float64Ptr(123.45),
		},
		{
			name:      "invalid type - object",
			input:     `{"id": 1}`,
			expectErr: true,
		},
		{
			name:      "invalid type - array",
			input:     `[1, 2, 3]`,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var id RequestID
			err := json.Unmarshal([]byte(tt.input), &id)

			if tt.expectErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			switch {
			case tt.expectNull:
				assert.True(t, id.IsNull())
			case tt.expectStr != nil:
				require.NotNil(t, id.String())
				assert.Equal(t, *tt.expectStr, *id.String())
			case tt.expectNum != nil:
				require.NotNil(t, id.Number())
				assert.Equal(t, *tt.expectNum, *id.Number())
			}
		})
	}
}

func TestRequest_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		req  Request
	}{
		{
			name: "with string id",
			req: Request{
				JSONRPC: "2.0",
				ID:      NewStringID("req-1"),
				Method:  "tools/list",
				Params:  json.RawMessage(`{}`),
			},
		},
		{
			name: "with number id",
			req: Request{
				JSONRPC: "2.0",
				ID:      NewNumberID(1),
				Method:  "tools/call",
				Params:  json.RawMessage(`{"name":"jira","arguments":{}}`),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := json.Marshal(tt.req)
			require.NoError(t, err)

			// Unmarshal
			var decoded Request
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err)

			// Compare
			assert.Equal(t, tt.req.JSONRPC, decoded.JSONRPC)
			assert.Equal(t, tt.req.Method, decoded.Method)
			assert.Equal(t, tt.req.Params, decoded.Params)

			// Compare IDs
			require.NotNil(t, decoded.ID)
			switch {
			case tt.req.ID.IsNull():
				assert.True(t, decoded.ID.IsNull())
			case tt.req.ID.String() != nil:
				require.NotNil(t, decoded.ID.String())
				assert.Equal(t, *tt.req.ID.String(), *decoded.ID.String())
			case tt.req.ID.Number() != nil:
				require.NotNil(t, decoded.ID.Number())
				assert.Equal(t, *tt.req.ID.Number(), *decoded.ID.Number())
			}
		})
	}
}

func TestResponse_WithError(t *testing.T) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      NewNumberID(1),
		Error: &ErrorObject{
			Code:    ErrorMethodNotFound,
			Message: "method 'foo' not found",
		},
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded Response
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "2.0", decoded.JSONRPC)
	assert.NotNil(t, decoded.Error)
	assert.Equal(t, ErrorMethodNotFound, decoded.Error.Code)
	assert.Equal(t, "method 'foo' not found", decoded.Error.Message)
	assert.Nil(t, decoded.Result)
}

func TestResponse_WithResult(t *testing.T) {
	result := ToolsListResult{
		Tools: []Tool{
			{
				Name:        "jira",
				Description: "Jira integration",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			},
		},
	}
	resultJSON, _ := json.Marshal(result)

	resp := Response{
		JSONRPC: "2.0",
		ID:      NewStringID("req-1"),
		Result:  resultJSON,
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded Response
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "2.0", decoded.JSONRPC)
	assert.Nil(t, decoded.Error)
	assert.NotNil(t, decoded.Result)

	var decodedResult ToolsListResult
	err = json.Unmarshal(decoded.Result, &decodedResult)
	require.NoError(t, err)
	assert.Len(t, decodedResult.Tools, 1)
	assert.Equal(t, "jira", decodedResult.Tools[0].Name)
}

// Helper functions.
func stringPtr(s string) *string {
	return &s
}

func float64Ptr(f float64) *float64 {
	return &f
}
