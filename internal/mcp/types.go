package mcp

import "encoding/json"

// jsonNull is the JSON literal for null values.
const jsonNull = "null"

// JSON-RPC 2.0 specification: https://www.jsonrpc.org/specification

// Request represents a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *RequestID      `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// UnmarshalJSON implements custom unmarshaling for Request to handle null IDs.
func (r *Request) UnmarshalJSON(data []byte) error {
	type Alias Request
	aux := &struct {
		IDRaw json.RawMessage `json:"id"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Handle the ID field specially
	if aux.IDRaw != nil {
		r.ID = &RequestID{}
		if err := r.ID.UnmarshalJSON(aux.IDRaw); err != nil {
			return err
		}
	}

	return nil
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *RequestID      `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ErrorObject    `json:"error,omitempty"`
}

// RequestID represents a JSON-RPC request ID.
// For requests, MCP expects string or integer IDs (notifications omit the id field).
// For responses, null can still be used per JSON-RPC error handling.
type RequestID struct {
	str    *string
	num    *float64
	isNull bool
}

// NewStringID creates a RequestID with a string value.
func NewStringID(s string) *RequestID {
	return &RequestID{str: &s}
}

// NewNumberID creates a RequestID with a number value.
func NewNumberID(n float64) *RequestID {
	return &RequestID{num: &n}
}

// NewNullID creates a RequestID with a null value.
func NewNullID() *RequestID {
	return &RequestID{isNull: true}
}

// String returns the string value if set.
func (r *RequestID) String() *string {
	return r.str
}

// Number returns the number value if set.
func (r *RequestID) Number() *float64 {
	return r.num
}

// IsNull returns true if the ID is null.
func (r *RequestID) IsNull() bool {
	return r.isNull
}

// MarshalJSON implements json.Marshaler for RequestID.
func (r RequestID) MarshalJSON() ([]byte, error) {
	if r.isNull {
		return []byte(jsonNull), nil
	}
	if r.str != nil {
		return json.Marshal(r.str)
	}
	if r.num != nil {
		return json.Marshal(r.num)
	}
	return []byte("null"), nil
}

// UnmarshalJSON implements json.Unmarshaler for RequestID.
func (r *RequestID) UnmarshalJSON(data []byte) error {
	// Check for null
	if string(data) == jsonNull {
		return &ErrorObject{
			Code:    ErrorInvalidRequest,
			Message: "id must be string or number",
		}
	}

	// Try to unmarshal as string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		r.str = &s
		return nil
	}

	// Try to unmarshal as number
	var n float64
	if err := json.Unmarshal(data, &n); err == nil {
		r.num = &n
		return nil
	}

	return &ErrorObject{
		Code:    ErrorInvalidRequest,
		Message: "id must be string or number",
	}
}

// ErrorObject represents a JSON-RPC 2.0 error object.
//
//nolint:errname // JSON-RPC spec uses the `ErrorObject` type name.
type ErrorObject struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error implements the error interface for ErrorObject.
func (e *ErrorObject) Error() string {
	return e.Message
}

// Standard JSON-RPC 2.0 error codes.
const (
	ErrorParseError     = -32700 // Parse error
	ErrorInvalidRequest = -32600 // Invalid Request
	ErrorMethodNotFound = -32601 // Method not found
	ErrorInvalidParams  = -32602 // Invalid params
	ErrorInternalError  = -32603 // Internal error
)

// Application-specific error codes (as defined in EPIC_6.md).
const (
	ErrorApprovalRequired  = -32001 // Approval required
	ErrorUnauthorized      = -32002 // Authentication failed
	ErrorDeniedByPolicy    = -32003 // Denied by policy
	ErrorPolicyInvalid     = -32004 // Policy evaluation error
	ErrorRateLimitExceeded = -32005 // Rate limit exceeded
	ErrorFileAccessDenied  = -32006 // File access denied
	ErrorQuotaExceeded     = -32007 // Usage quota exceeded
)

// MCP method names.
const (
	MethodInitialize               = "initialize"
	MethodNotificationsInitialized = "notifications/initialized"
	MethodToolsList                = "tools/list"
	MethodToolsCall                = "tools/call"
	MethodResourcesList            = "resources/list"
	MethodResourcesRead            = "resources/read"
)

// MCP protocol versions.
const (
	ProtocolVersion20251125 = "2025-11-25"
)

// ToolsListResult represents the result of a tools/list call.
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// Tool represents an MCP tool definition.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// Implementation describes MCP client/server implementation metadata.
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeParams represents the params for an initialize request.
type InitializeParams struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    json.RawMessage `json:"capabilities,omitempty"`
	ClientInfo      *Implementation `json:"clientInfo,omitempty"`
}

// InitializeResult represents the result for an initialize response.
type InitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      Implementation `json:"serverInfo"`
}

// ToolsCallParams represents the params for a tools/call request.
type ToolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolsCallResult represents the result of a tools/call request.
type ToolsCallResult struct {
	Content []Content `json:"content"`
}

// Content represents a content item in the tool response.
type Content struct {
	Type string          `json:"type"`
	Text string          `json:"text,omitempty"`
	JSON json.RawMessage `json:"json,omitempty"`
}

// ApprovalRequiredData represents the data for an approval required error.
type ApprovalRequiredData struct {
	ApprovalRequired  bool     `json:"approval_required"`
	ApprovalRequestID string   `json:"approval_request_id"`
	ExpiresAt         string   `json:"expires_at"`
	UIURL             string   `json:"ui_url,omitempty"`
	PolicyVersion     string   `json:"policy_version"`
	RuleID            string   `json:"rule_id"`
	Risk              string   `json:"risk"`
	Reasons           []string `json:"reasons"`
}

// DeniedData represents the data for a denied error.
type DeniedData struct {
	Denied        bool     `json:"denied"`
	PolicyVersion string   `json:"policy_version"`
	RuleID        string   `json:"rule_id"`
	Risk          string   `json:"risk"`
	Reasons       []string `json:"reasons"`
	Tags          []string `json:"tags,omitempty"`
}

// Resource represents an MCP resource definition.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
}

// ResourcesListResult represents the result of a resources/list call.
type ResourcesListResult struct {
	Resources []Resource `json:"resources"`
}

// ResourcesReadParams represents the params for a resources/read request.
type ResourcesReadParams struct {
	URI string `json:"uri"`
}

// ResourceContent represents a single content item in a resource read response.
type ResourceContent struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

// ResourcesReadResult represents the result of a resources/read call.
type ResourcesReadResult struct {
	Contents []ResourceContent `json:"contents"`
}
