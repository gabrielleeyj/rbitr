package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/mcp"
)

// MCPClient forwards MCP JSON-RPC requests to upstream MCP servers.
type MCPClient struct {
	client  *http.Client
	timeout time.Duration
}

const (
	defaultTimeout  = 30 * time.Second
	maxResponseSize = 10 * 1024 * 1024 // 10MB
)

// NewMCPClient creates a new MCP client with the specified timeout.
func NewMCPClient(timeout time.Duration) *MCPClient {
	if timeout == 0 {
		timeout = defaultTimeout
	}
	return &MCPClient{
		client: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
	}
}

// ForwardRequest forwards an MCP JSON-RPC request to the upstream server.
// It preserves the request ID and returns the upstream response.
func (m *MCPClient) ForwardRequest(ctx context.Context, upstreamURL string, req *mcp.Request) (*mcp.Response, error) {
	if err := validateOutboundURL(upstreamURL); err != nil {
		return nil, err
	}

	// Marshal the request to JSON
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal MCP request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Add("Accept", "application/json")
	httpReq.Header.Add("Accept", "text/event-stream")

	// Execute request
	//nolint:gosec // G704: upstream URL is tenant-admin configured and validated before execution.
	httpResp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer httpResp.Body.Close()

	// Read response body (with size limit to prevent memory exhaustion)
	limitedReader := io.LimitReader(httpResp.Body, maxResponseSize+1)
	respBody, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read upstream response: %w", err)
	}

	// Check if we hit the size limit
	if len(respBody) > maxResponseSize {
		return nil, fmt.Errorf("upstream response exceeded maximum size of %d bytes", maxResponseSize)
	}

	// Handle non-200 HTTP status codes
	if httpResp.StatusCode != http.StatusOK {
		// Try to parse as JSON-RPC error response
		var mcpResp mcp.Response
		if err := json.Unmarshal(respBody, &mcpResp); err == nil && mcpResp.Error != nil {
			// Valid JSON-RPC error response from upstream
			return &mcpResp, nil
		}
		// Not a valid JSON-RPC error, wrap it
		return &mcp.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &mcp.ErrorObject{
				Code:    mcp.ErrorInternalError,
				Message: fmt.Sprintf("upstream returned HTTP %d", httpResp.StatusCode),
			},
		}, nil
	}

	// Parse successful response
	var mcpResp mcp.Response
	if err := json.Unmarshal(respBody, &mcpResp); err != nil {
		return nil, fmt.Errorf("failed to parse upstream JSON-RPC response: %w", err)
	}

	// Verify response ID matches request ID (JSON-RPC protocol requirement)
	if req.ID != nil && !req.ID.IsNull() {
		if mcpResp.ID == nil || mcpResp.ID.IsNull() {
			return nil, errors.New("upstream response missing ID for request with ID")
		}
		// Compare IDs
		if !compareRequestIDs(req.ID, mcpResp.ID) {
			return nil, errors.New("upstream response ID mismatch")
		}
	}

	return &mcpResp, nil
}

// compareRequestIDs compares two RequestIDs for equality.
func compareRequestIDs(a, b *mcp.RequestID) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.IsNull() && b.IsNull() {
		return true
	}
	if a.IsNull() != b.IsNull() {
		return false
	}

	aStr := a.String()
	bStr := b.String()
	if aStr != nil && bStr != nil {
		return *aStr == *bStr
	}

	aNum := a.Number()
	bNum := b.Number()
	if aNum != nil && bNum != nil {
		return *aNum == *bNum
	}

	return false
}
