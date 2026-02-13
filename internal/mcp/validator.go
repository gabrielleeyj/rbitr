package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// MaxRequestSize is the maximum size of an MCP request body (10MB).
const MaxRequestSize = 10 * 1024 * 1024

// ValidateAndParseRequest validates and parses a JSON-RPC request from raw bytes.
func ValidateAndParseRequest(data []byte, maxSize int64) (*Request, error) {
	if maxSize <= 0 {
		maxSize = MaxRequestSize
	}

	// Check size limit
	if int64(len(data)) > maxSize {
		return nil, &ErrorObject{
			Code:    ErrorInvalidRequest,
			Message: "request body too large",
		}
	}

	var req Request
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&req); err != nil {
		// Check if error is already an ErrorObject (e.g., from RequestID.UnmarshalJSON)
		var errObj *ErrorObject
		if errors.As(err, &errObj) {
			return nil, errObj
		}
		// Otherwise it's a parse error
		return nil, &ErrorObject{
			Code:    ErrorParseError,
			Message: "parse error",
		}
	}

	// Check for trailing data (extra JSON values)
	if decoder.More() {
		return nil, &ErrorObject{
			Code:    ErrorInvalidRequest,
			Message: "request must contain exactly one JSON value",
		}
	}

	// Validate JSON-RPC version
	if req.JSONRPC != "2.0" {
		return nil, &ErrorObject{
			Code:    ErrorInvalidRequest,
			Message: "jsonrpc must be '2.0'",
		}
	}

	// Validate method is present
	if req.Method == "" {
		return nil, &ErrorObject{
			Code:    ErrorInvalidRequest,
			Message: "method is required",
		}
	}

	return &req, nil
}

// NewSuccessResponse creates a JSON-RPC success response.
func NewSuccessResponse(id *RequestID, result interface{}) (*Response, error) {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  resultJSON,
	}, nil
}

// NewErrorResponse creates a JSON-RPC error response.
func NewErrorResponse(id *RequestID, errObj *ErrorObject) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   errObj,
	}
}

// NewMethodNotFoundError creates a method not found error.
func NewMethodNotFoundError(method string) *ErrorObject {
	return &ErrorObject{
		Code:    ErrorMethodNotFound,
		Message: fmt.Sprintf("method '%s' not found", method),
	}
}

// NewInvalidParamsError creates an invalid params error.
func NewInvalidParamsError(msg string) *ErrorObject {
	return &ErrorObject{
		Code:    ErrorInvalidParams,
		Message: fmt.Sprintf("invalid params: %s", msg),
	}
}

// NewInternalError creates an internal error.
func NewInternalError(msg string) *ErrorObject {
	return &ErrorObject{
		Code:    ErrorInternalError,
		Message: msg,
	}
}

// NewApprovalRequiredError creates an approval required error.
func NewApprovalRequiredError(data *ApprovalRequiredData) *ErrorObject {
	dataJSON, _ := json.Marshal(data)
	return &ErrorObject{
		Code:    ErrorApprovalRequired,
		Message: "approval required",
		Data:    dataJSON,
	}
}

// NewDeniedError creates a denied by policy error.
func NewDeniedError(data *DeniedData) *ErrorObject {
	dataJSON, _ := json.Marshal(data)
	return &ErrorObject{
		Code:    ErrorDeniedByPolicy,
		Message: "denied by policy",
		Data:    dataJSON,
	}
}

// PolicyErrorCode represents safe, predefined policy error codes.
type PolicyErrorCode string

const (
	PolicyErrorInvalidOutput  PolicyErrorCode = "POLICY_INVALID_OUTPUT"
	PolicyErrorEvalFailed     PolicyErrorCode = "POLICY_EVAL_FAILED"
	PolicyErrorNotFound       PolicyErrorCode = "POLICY_NOT_FOUND"
	PolicyErrorSchemaViolation PolicyErrorCode = "POLICY_SCHEMA_VIOLATION"
)

// NewPolicyInvalidError creates a policy evaluation error with a safe error code.
func NewPolicyInvalidError(code PolicyErrorCode) *ErrorObject {
	data := map[string]string{"reason_code": string(code)}
	dataJSON, _ := json.Marshal(data)
	return &ErrorObject{
		Code:    ErrorPolicyInvalid,
		Message: "policy evaluation error",
		Data:    dataJSON,
	}
}
