package admin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateToolID(t *testing.T) {
	valid := []string{
		"abc",
		"my_tool",
		"my-tool",
		"Tool123",
		"a23456789012345678901234567890123456789012345678901234567890_end",
	}
	for _, id := range valid {
		require.NoError(t, validateToolID(id), "expected valid: %s", id)
	}

	invalid := []string{
		"",
		"ab",
		"-abc",
		"_abc",
		"a b",
		"a.b",
		"a/b",
		// 65 chars — too long
		"a2345678901234567890123456789012345678901234567890123456789012345",
	}
	for _, id := range invalid {
		require.Error(t, validateToolID(id), "expected invalid: %q", id)
	}
}

func TestValidateAuthType(t *testing.T) {
	for _, at := range []string{"none", "bearer", "api_key", "oauth2_client_credentials"} {
		require.NoError(t, validateAuthType(at))
	}
	require.Error(t, validateAuthType("magic"))
	require.Error(t, validateAuthType(""))
}

func TestValidateTransport(t *testing.T) {
	require.NoError(t, validateTransport("http"))
	require.NoError(t, validateTransport("mcp"))
	require.Error(t, validateTransport("grpc"))
	require.Error(t, validateTransport(""))
}

func TestValidateInputSchemaJSON(t *testing.T) {
	// nil/empty is ok
	require.NoError(t, validateInputSchemaJSON(nil))
	require.NoError(t, validateInputSchemaJSON(json.RawMessage{}))

	// valid object
	require.NoError(t, validateInputSchemaJSON(json.RawMessage(`{"type":"object"}`)))

	// not an object
	require.Error(t, validateInputSchemaJSON(json.RawMessage(`"string"`)))
	require.Error(t, validateInputSchemaJSON(json.RawMessage(`[1,2]`)))
	require.Error(t, validateInputSchemaJSON(json.RawMessage(`invalid`)))
}
