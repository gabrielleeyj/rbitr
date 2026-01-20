package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStringArrayScan(t *testing.T) {
	cases := []struct {
		name      string
		input     any
		expected  []string
		expectErr bool
	}{
		{
			name:     "nil",
			input:    nil,
			expected: nil,
		},
		{
			name:     "slice",
			input:    []string{"a", "b"},
			expected: []string{"a", "b"},
		},
		{
			name:     "empty array string",
			input:    "{}",
			expected: []string{},
		},
		{
			name:     "array string",
			input:    "{admin:write,admin:read}",
			expected: []string{"admin:write", "admin:read"},
		},
		{
			name:     "quoted array string",
			input:    "{\"admin:write\",\"admin:read\"}",
			expected: []string{"admin:write", "admin:read"},
		},
		{
			name:     "byte array",
			input:    []byte("{alpha,beta}"),
			expected: []string{"alpha", "beta"},
		},
		{
			name:      "unsupported",
			input:     123,
			expectErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var arr StringArray
			err := arr.Scan(tc.input)
			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, []string(arr))
		})
	}
}

func TestStringArrayValue(t *testing.T) {
	arr := StringArray{"one", "two"}
	value, err := arr.Value()
	require.NoError(t, err)
	require.Equal(t, `{"one","two"}`, value)
}
