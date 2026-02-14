package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractRequestID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expectID bool
		checkID  func(*testing.T, *RequestID)
	}{
		{
			name:     "valid request with string id",
			input:    `{"jsonrpc":"2.0","id":"req-123","method":"test"}`,
			expectID: true,
			checkID: func(t *testing.T, id *RequestID) {
				require.NotNil(t, id.String())
				assert.Equal(t, "req-123", *id.String())
			},
		},
		{
			name:     "valid request with number id",
			input:    `{"jsonrpc":"2.0","id":456,"method":"test"}`,
			expectID: true,
			checkID: func(t *testing.T, id *RequestID) {
				require.NotNil(t, id.Number())
				assert.Equal(t, float64(456), *id.Number())
			},
		},
		{
			name:     "null id is invalid and not extracted",
			input:    `{"jsonrpc":"2.0","id":null,"method":"test"}`,
			expectID: false,
		},
		{
			name:     "invalid jsonrpc version but valid id",
			input:    `{"jsonrpc":"1.0","id":789,"method":"test"}`,
			expectID: true,
			checkID: func(t *testing.T, id *RequestID) {
				require.NotNil(t, id.Number())
				assert.Equal(t, float64(789), *id.Number())
			},
		},
		{
			name:     "missing method but valid id",
			input:    `{"jsonrpc":"2.0","id":"abc"}`,
			expectID: true,
			checkID: func(t *testing.T, id *RequestID) {
				require.NotNil(t, id.String())
				assert.Equal(t, "abc", *id.String())
			},
		},
		{
			name:     "no id field",
			input:    `{"jsonrpc":"2.0","method":"test"}`,
			expectID: false,
		},
		{
			name:     "invalid json",
			input:    `{invalid json`,
			expectID: false,
		},
		{
			name:     "id is invalid type (object)",
			input:    `{"jsonrpc":"2.0","id":{"nested":"object"},"method":"test"}`,
			expectID: false,
		},
		{
			name:     "id is invalid type (array)",
			input:    `{"jsonrpc":"2.0","id":[1,2,3],"method":"test"}`,
			expectID: false,
		},
		{
			name:     "empty json",
			input:    `{}`,
			expectID: false,
		},
		{
			name:     "trailing json data with valid id",
			input:    `{"jsonrpc":"2.0","id":123,"method":"tools/list"}{"extra":1}`,
			expectID: true,
			checkID: func(t *testing.T, id *RequestID) {
				require.NotNil(t, id.Number())
				assert.Equal(t, float64(123), *id.Number())
			},
		},
		{
			name:     "trailing json data with string id",
			input:    `{"jsonrpc":"2.0","id":"req-999","method":"test"}{"extra":"data","more":true}`,
			expectID: true,
			checkID: func(t *testing.T, id *RequestID) {
				require.NotNil(t, id.String())
				assert.Equal(t, "req-999", *id.String())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := ExtractRequestID([]byte(tt.input))

			if tt.expectID {
				require.NotNil(t, id, "expected to extract ID")
				if tt.checkID != nil {
					tt.checkID(t, id)
				}
			} else {
				assert.Nil(t, id, "expected nil ID")
			}
		})
	}
}

func TestExtractRequestID_PreservesIDEvenWithOtherErrors(t *testing.T) {
	// This is the key use case: even if the request has other validation errors,
	// we should still extract the ID for proper error response correlation

	testCases := []struct {
		name  string
		input string
		idVal interface{}
	}{
		{
			name:  "wrong jsonrpc version with string id",
			input: `{"jsonrpc":"1.0","id":"client-123","method":"tools/list"}`,
			idVal: "client-123",
		},
		{
			name:  "missing method with number id",
			input: `{"jsonrpc":"2.0","id":999}`,
			idVal: float64(999),
		},
		{
			name:  "empty method with string id",
			input: `{"jsonrpc":"2.0","id":"empty-method","method":""}`,
			idVal: "empty-method",
		},
		{
			name:  "extra fields with valid id",
			input: `{"jsonrpc":"2.0","id":42,"method":"test","extra":"ignored","more":true}`,
			idVal: float64(42),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			id := ExtractRequestID([]byte(tc.input))
			require.NotNil(t, id, "should extract ID despite other validation errors")

			switch expected := tc.idVal.(type) {
			case string:
				require.NotNil(t, id.String())
				assert.Equal(t, expected, *id.String())
			case float64:
				require.NotNil(t, id.Number())
				assert.Equal(t, expected, *id.Number())
			}
		})
	}
}
