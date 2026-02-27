package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/mcp"
)

func TestMCPClient_ForwardRequest_Success(t *testing.T) {
	// Create mock upstream MCP server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		acceptHeader := strings.Join(r.Header.Values("Accept"), ",")
		assert.Contains(t, acceptHeader, "application/json")
		assert.Contains(t, acceptHeader, "text/event-stream")

		// Read request
		var req mcp.Request
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Verify request contents
		assert.Equal(t, "2.0", req.JSONRPC)
		assert.Equal(t, "tools/call", req.Method)

		// Return success response
		resultData, _ := json.Marshal(mcp.ToolsCallResult{
			Content: []mcp.Content{
				{Type: "text", Text: "Tool executed successfully"},
			},
		})
		resp := mcp.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  resultData,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))
	defer upstream.Close()

	// Create MCP client
	client := NewMCPClient(5 * time.Second)

	// Create test request
	reqID := mcp.NewStringID("test-1")
	params, _ := json.Marshal(map[string]any{
		"name":      "test-tool",
		"arguments": map[string]any{"action": "test"},
	})
	req := &mcp.Request{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  "tools/call",
		Params:  params,
	}

	// Forward request
	ctx := context.Background()
	resp, err := client.ForwardRequest(ctx, upstream.URL, req)

	// Verify response
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "2.0", resp.JSONRPC)
	assert.NotNil(t, resp.ID)
	assert.Nil(t, resp.Error)
	assert.NotNil(t, resp.Result)

	// Verify result content
	var result mcp.ToolsCallResult
	err = json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)
	assert.Len(t, result.Content, 1)
	assert.Equal(t, "text", result.Content[0].Type)
	assert.Equal(t, "Tool executed successfully", result.Content[0].Text)
}

func TestMCPClient_ForwardRequest_UpstreamError(t *testing.T) {
	// Create mock upstream MCP server that returns an error
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req mcp.Request
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		resp := mcp.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &mcp.ErrorObject{
				Code:    mcp.ErrorInvalidParams,
				Message: "invalid parameters",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))
	defer upstream.Close()

	client := NewMCPClient(5 * time.Second)

	reqID := mcp.NewStringID("test-2")
	req := &mcp.Request{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  "tools/call",
		Params:  json.RawMessage(`{}`),
	}

	ctx := context.Background()
	resp, err := client.ForwardRequest(ctx, upstream.URL, req)

	// Should not return error, but response should contain error object
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, mcp.ErrorInvalidParams, resp.Error.Code)
	assert.Equal(t, "invalid parameters", resp.Error.Message)
}

func TestMCPClient_ForwardRequest_UpstreamHTTPError(t *testing.T) {
	// Create mock upstream that returns HTTP 500
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, err := w.Write([]byte("Internal Server Error"))
		require.NoError(t, err)
	}))
	defer upstream.Close()

	client := NewMCPClient(5 * time.Second)

	reqID := mcp.NewStringID("test-3")
	req := &mcp.Request{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  "tools/call",
		Params:  json.RawMessage(`{}`),
	}

	ctx := context.Background()
	resp, err := client.ForwardRequest(ctx, upstream.URL, req)

	// Should return response with error, not Go error
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, mcp.ErrorInternalError, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "500")
}

func TestMCPClient_ForwardRequest_Timeout(t *testing.T) {
	// Create slow upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	// Create client with short timeout
	client := NewMCPClient(100 * time.Millisecond)

	reqID := mcp.NewStringID("test-4")
	req := &mcp.Request{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  "tools/call",
		Params:  json.RawMessage(`{}`),
	}

	ctx := context.Background()
	_, err := client.ForwardRequest(ctx, upstream.URL, req)

	// Should return timeout error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upstream request failed")
}

func TestMCPClient_ForwardRequest_InvalidUpstreamURL(t *testing.T) {
	client := NewMCPClient(5 * time.Second)
	reqID := mcp.NewStringID("test-invalid-url")
	req := &mcp.Request{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  "tools/call",
		Params:  json.RawMessage(`{}`),
	}

	_, err := client.ForwardRequest(context.Background(), "ftp://example", req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid outbound URL")
}

func TestMCPClient_ForwardRequest_IDMismatch(t *testing.T) {
	// Create upstream that returns wrong ID
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return different ID than request
		wrongID := mcp.NewStringID("wrong-id")
		resp := mcp.Response{
			JSONRPC: "2.0",
			ID:      wrongID,
			Result:  json.RawMessage(`{}`),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))
	defer upstream.Close()

	client := NewMCPClient(5 * time.Second)

	reqID := mcp.NewStringID("test-5")
	req := &mcp.Request{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  "tools/call",
		Params:  json.RawMessage(`{}`),
	}

	ctx := context.Background()
	_, err := client.ForwardRequest(ctx, upstream.URL, req)

	// Should return ID mismatch error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ID mismatch")
}

func TestMCPClient_ForwardRequest_OversizedResponse(t *testing.T) {
	// Create upstream that returns huge response (>10MB)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Write more than 10MB of data
		largeData := strings.Repeat("A", 11*1024*1024)
		_, err := w.Write([]byte(largeData))
		require.NoError(t, err)
	}))
	defer upstream.Close()

	client := NewMCPClient(30 * time.Second)

	reqID := mcp.NewStringID("test-6")
	req := &mcp.Request{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  "tools/call",
		Params:  json.RawMessage(`{}`),
	}

	ctx := context.Background()
	_, err := client.ForwardRequest(ctx, upstream.URL, req)

	// Should return size limit error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeded maximum size")
}

func TestMCPClient_ForwardRequest_ExactSizeLimitAllowed(t *testing.T) {
	const maxResponseSize = 10 * 1024 * 1024

	reqID := mcp.NewStringID("test-limit")
	prefix := `{"jsonrpc":"2.0","id":"test-limit","result":"`
	suffix := `"}`
	paddingLen := maxResponseSize - len(prefix) - len(suffix)
	require.Greater(t, paddingLen, 0)
	payload := prefix + strings.Repeat("x", paddingLen) + suffix
	require.Equal(t, maxResponseSize, len(payload))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload))
	}))
	defer upstream.Close()

	client := NewMCPClient(30 * time.Second)
	req := &mcp.Request{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  "tools/call",
		Params:  json.RawMessage(`{}`),
	}

	resp, err := client.ForwardRequest(context.Background(), upstream.URL, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Nil(t, resp.Error)
}
